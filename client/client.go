package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shockerjue/gffg/common"
	"github.com/shockerjue/gffg/metrics"
	"github.com/shockerjue/gffg/proto"
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
	group string
	p     *pool
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
	return &Client{
		group: group,
		p:     newPool(opt.registry),
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
		zzlog.Warnw("call success", zap.Any("method", rpc), zap.Any("cost",
			fmt.Sprintf("%dms", time.Now().UnixMilli()-startAt.UnixMilli())), zap.Any("rpcCpde", rpcCode),
			zap.Any("push", strings.Replace(rpc, ".", "_", -1)), zap.String("traceId", traceId))

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
		return nil, errors.New(
			fmt.Sprintf("call.Marshal error[%s]	traceId:%s", err.Error(), traceId))
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

		return nil, errors.New(
			fmt.Sprintf("call.Write error[%s]	traceId:%s", err.Error(), traceId))
	}

	if opt.onlyCall {
		rpcCode = 0

		return make([]byte, 0), nil
	}

	// async handle
	select {
	case <-time.After(time.Second * time.Duration(opt.timeout)):
		c.p.wrw.Lock()
		close(c.p.callItem[Sid].Ch)
		delete(c.p.callItem, Sid)

		c.p.wrw.Unlock()

		rpcCode = 408
		return nil, errors.New(fmt.Sprintf("Wait fail , timeout	 traceId:%s", &traceId))

	case <-waitCh:
		c.p.wrw.Lock()
		packet = c.p.callItem[Sid].Packet
		rpcCode = c.p.callItem[Sid].Code
		c.p.callItem[Sid].Packet = nil

		close(c.p.callItem[Sid].Ch)
		delete(c.p.callItem, Sid)

		c.p.wrw.Unlock()

		break
	}

	if 0 != rpcCode {
		return nil, errors.New(fmt.Sprintf(
			"Request to server failed! called code:%d	traceId:%s",
			rpcCode, common.GetTraceId(ctx)))
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

			zzlog.Errorw("Client.Call error", zap.String("method", req.m), zap.Error(r.(error)))
			return
		}

		metrics.Counter("client", req.m)
		if nil != err {
			metrics.Counter("client", fmt.Sprintf("%s.error", req.m))
		}
	}()
	packet, err := req.in.Marshal()
	if nil != err {
		return nil, err
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
		zzlog.Errorw("client.Call error", zap.Any("group", c.group),
			zap.Any("name", req.name), zap.Any("req.m", req.m),
			zap.String("traceId", common.GetTraceId(ctx)), zap.Error(err))

		c.p.removeByClient(cli.Group, cli.Svrname, cli.Name, cli.S.Request().RemoteAddr().String())
	}

	return res, err
}

func (c *Client) Destroy() {
	c.p.destroy()
}
