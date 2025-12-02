package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shockerjue/gffg/circuitbreaker"
	"github.com/shockerjue/gffg/common"
	"github.com/shockerjue/gffg/metrics"
	"github.com/shockerjue/gffg/proto"
	"github.com/shockerjue/gffg/ratelimit"
	"github.com/shockerjue/gffg/registry"
	"github.com/shockerjue/gffg/transport"
	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

type Request struct {
	name string
	in   common.Message
	m    string
}

type Client struct {
	group          string
	p              *pool
	circuitBreaker circuitbreaker.CircuitBreaker
	rateLimiter    ratelimit.Limiter
}

// Create RPC Client
//
// @param	group 	rpc server group
// @param 	opt 	create option
//
//	client.Registry(...) Use Custom registry
func NewClient(group string, opts ...ClientOption) *Client {
	var opt Options
	for _, o := range opts {
		o(&opt)
	}
	if opt.registry == nil {
		opt.registry = registry.Registry()
	}

	// Create circuit breaker
	var cb circuitbreaker.CircuitBreaker
	if opt.circuitBreaker != nil {
		cb = opt.circuitBreaker
	} else {
		cbConfig := circuitbreaker.DefaultConfig(fmt.Sprintf("client-%s", group))
		if createdCB, err := circuitbreaker.NewCircuitBreaker(cbConfig); err == nil {
			cb = createdCB
		} else {
			zzlog.Errorw("failed to create circuit breaker", zap.Error(err))
		}
	}

	// Create rate limiter
	var rl ratelimit.Limiter
	if opt.rateLimiter != nil {
		rl = opt.rateLimiter
	} else {
		rlConfig := &ratelimit.Config{
			DefaultLimit: ratelimit.Limit{
				Rate:      1000,
				Burst:     2000,
				Period:    time.Second,
				Algorithm: ratelimit.TokenBucket,
			},
			StorageBackend: ratelimit.LocalStorage,
			LocalConfig: &ratelimit.LocalConfig{
				MaxKeys:         10000,
				CleanupInterval: 5 * time.Minute,
			},
			CleanupInterval: 5 * time.Minute,
			MetricsEnabled:  true,
			DynamicLimits:   false,
		}
		if limiter, err := ratelimit.NewTokenBucket(rlConfig); err == nil {
			rl = limiter
		} else {
			zzlog.Errorw("failed to create rate limiter", zap.Error(err))
		}
	}

	return &Client{
		group:          group,
		p:              newPool(opt.registry),
		circuitBreaker: cb,
		rateLimiter:    rl,
	}
}

// Create RPC Request
//
// @param 	name	Call RPC server name
// @param 	m		Call RPC method name
// @param 	in		Call RPC request body
func (c *Client) NewRequest(name string, m string, in common.Message) *Request {
	return &Request{
		name: name,
		in:   in,
		m:    m,
	}
}

func (c *Client) call(ctx context.Context, res *transport.Response, rpc string,
	packet []byte, opts ...CallOption) ([]byte, error) {
	rpcCode := int32(-1)
	startAt := time.Now()
	traceId := common.GetTraceId(ctx)
	defer func() {
		// Only log warnings for errors or if debug is enabled
		if rpcCode != 0 || zzlog.IsDebugEnabled() {
			cost := time.Now().UnixMilli() - startAt.UnixMilli()
			if rpcCode != 0 || cost > 1000 { // Log slow calls (>1s) even if successful
				zzlog.Warnw("call completed", zap.String("method", rpc), zap.Int64("cost_ms", cost),
					zap.Int32("rpcCode", rpcCode), zap.String("traceId", traceId))
			} else if zzlog.IsDebugEnabled() {
				zzlog.Debugw("call success", zap.String("method", rpc), zap.Int64("cost_ms", cost),
					zap.Int32("rpcCode", rpcCode), zap.String("traceId", traceId))
			}
		}
		metrics.MethodCode(rpc, fmt.Sprintf("%d", rpcCode))
	}()

	opt := initOpt(opts...)
	Sid := Sid()

	header := make(map[string]string)
	header["traceId"] = common.GetTraceId(ctx)
	if opt.onlyCall {
		header["onlyCall"] = "1"
	}

	data := &proto.Request{
		Sid:     Sid,
		Headers: header,
		RpcId:   int64(common.GenRid(rpc)),
		Packet:  packet,
	}

	req, err := data.Marshal()
	if nil != err {
		return nil, common.ErrRPCMarshal(traceId, err)
	}

	waitCh := make(chan int)
	defer func() {
		if !common.ClosedChanInt(waitCh) {
			close(waitCh)
		}
	}()

	if !opt.onlyCall {
		cc := &CallCond{
			Ch: waitCh,
		}

		c.p.wrw.Lock()
		c.p.callItem[Sid] = cc
		c.p.wrw.Unlock()
	}

	_, err = res.Write(req)
	if nil != err {
		rpcCode = 500

		return nil, common.NewRPCErrorWithErr(common.ErrCodeInternal, "Failed to write request", traceId, err)
	}

	if opt.onlyCall {
		rpcCode = 0

		return make([]byte, 0), nil
	}

	// async handle
	select {
	case <-time.After(time.Second * time.Duration(opt.timeout)):
		c.p.wrw.Lock()
		if item, exists := c.p.callItem[Sid]; exists {
			if !common.ClosedChanInt(item.Ch) {
				close(item.Ch)
			}
			delete(c.p.callItem, Sid)
		}
		c.p.wrw.Unlock()

		rpcCode = common.ErrCodeTimeout
		return nil, common.ErrRPCTimeout(traceId)

	case <-waitCh:
		c.p.wrw.Lock()
		if item, exists := c.p.callItem[Sid]; exists {
			packet = item.Packet
			rpcCode = item.Code
			item.Packet = nil

			if !common.ClosedChanInt(item.Ch) {
				close(item.Ch)
			}
			delete(c.p.callItem, Sid)
		} else {
			// This should not happen, but handle gracefully
			rpcCode = common.ErrCodeInternal
			packet = nil
		}
		c.p.wrw.Unlock()

		break
	}

	if 0 != rpcCode {
		return nil, common.NewRPCError(rpcCode, "Request to server failed", traceId)
	}

	return packet, nil
}

// Send an RPC request to the service
//
// @param	ctx 	call context
// @param	req 	*Request Object requesting RPC service
// @param	in		Request message body
// @param	opts 	Requested extended configuration
func (c *Client) Call(ctx context.Context, req *Request, in common.Message,
	opts ...CallOption) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			metrics.Counter("client", "panic")

			var err error
			switch v := r.(type) {
			case error:
				err = v
			case string:
				err = errors.New(v)
			default:
				err = fmt.Errorf("panic recovered: %v", v)
			}
			zzlog.Errorw("Client.Call error", zap.String("method", req.m), zap.Error(err))
			return
		}

		metrics.Counter("client", req.m)
		if nil != err {
			metrics.Counter("client", fmt.Sprintf("%s.error", req.m))
		}
	}()

	// Check rate limiter if available
	if c.rateLimiter != nil {
		key := fmt.Sprintf("%s:%s:%s", c.group, req.name, req.m)
		allowed, rlErr := c.rateLimiter.Allow(ctx, key)
		if rlErr != nil {
			zzlog.Warnw("rate limiter error",
				zap.String("method", req.m),
				zap.Error(rlErr))
		} else if !allowed {
			return nil, common.NewRPCError(429, "rate limit exceeded", common.GetTraceId(ctx))
		}
	}

	// Execute with circuit breaker if available
	if c.circuitBreaker != nil {
		var result interface{}
		result, err = c.circuitBreaker.Execute(ctx, func() (interface{}, error) {
			return c.executeCall(ctx, req, in, opts...)
		})
		if result != nil {
			res = result.([]byte)
		}
	} else {
		// Execute without circuit breaker
		res, err = c.executeCall(ctx, req, in, opts...)
	}

	return res, err
}

// executeCall executes the actual RPC call
func (c *Client) executeCall(ctx context.Context, req *Request, in common.Message,
	opts ...CallOption) (res []byte, err error) {
	packet, err := req.in.Marshal()
	if nil != err {
		return nil, common.ErrRPCMarshal(common.GetTraceId(ctx), err)
	}

	cli, err := c.p.response(ctx, c.group, req.name)
	if nil != err {
		return nil, err
	}

	ctx = context.WithValue(ctx, "instance", cli.instance)
	ctx = common.SetTraceId(ctx, common.GenUid())
	res, err = c.call(ctx, cli.S.Response(), req.m, packet, opts...)
	if nil != err && (strings.Contains(err.Error(), "closed") ||
		strings.Contains(err.Error(), "broken pipe")) {
		// Log connection errors at warn level, other errors at error level
		if strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "broken pipe") {
			zzlog.Warnw("client connection error", zap.String("group", c.group),
				zap.String("name", req.name), zap.String("method", req.m),
				zap.String("traceId", common.GetTraceId(ctx)), zap.Error(err))
		} else {
			zzlog.Errorw("client.Call error", zap.String("group", c.group),
				zap.String("name", req.name), zap.String("method", req.m),
				zap.String("traceId", common.GetTraceId(ctx)), zap.Error(err))
		}

		c.p.removeByClient(cli.Group, cli.Svrname, cli.Name, cli.S.Request().RemoteAddr().String())
	}

	return res, err
}

func (c *Client) Destroy() {
	c.p.destroy()

	// Close circuit breaker if available
	if c.circuitBreaker != nil {
		c.circuitBreaker.Close()
	}

	// Close rate limiter if available
	if c.rateLimiter != nil {
		c.rateLimiter.Close()
	}
}
