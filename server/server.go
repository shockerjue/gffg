package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/shockerjue/gffg/common"
	"github.com/shockerjue/gffg/config"
	"github.com/shockerjue/gffg/metrics"
	"github.com/shockerjue/gffg/proto"
	"github.com/shockerjue/gffg/registry"
	"github.com/shockerjue/gffg/transport"
	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"

	"github.com/StabbyCutyou/buffstreams"
)

type RequetChannel struct {
	Ctx context.Context
	Req *transport.Request
	Res *transport.Response
}

type Server struct {
	registry   registry.IRegistry
	sock       *transport.Listener
	rpcHandler *rpcHandler
	ctx        context.Context
	cancelFunc context.CancelFunc

	reqs       int64 // Number of requests being processing
	conns      int64 // Current number of connections
	addrs      string
	coroutines int

	reqCh chan RequetChannel
}

func NewServer(conf_file string, opts ...ServerOption) *Server {
	config.Init(conf_file)

	zzlog.Init(
		zzlog.WithLogName(config.Get("log", "log_file").String("")),
		zzlog.WithLevel(config.Get("log", "level").String("info")))

	var opt options
	for _, o := range opts {
		o(&opt)
	}
	if opt.registry == nil {
		rgis := registry.Registry()
		opt.registry = rgis
	}

	opt.registry.Provider(registry.Node(
		registry.Version(config.Get("server", "version").String("v0.0.1")),
		registry.Group(config.Get("server", "group").String("")),
		registry.Name(config.Get("server", "name").String("")),
		registry.Token(config.Get("server", "token").String("")),
		registry.Region(config.Get("server", "location", "region").String("")),
		registry.Zone(config.Get("server", "location", "zone").String("")),
		registry.Campus(config.Get("server", "location", "campus").String(""))))

	ctx, cFunc := context.WithCancel(context.Background())
	return &Server{
		ctx:        ctx,
		cancelFunc: cFunc,
		registry:   opt.registry,
		coroutines: config.Get("server", "coroutines").Int(32),
		reqCh:      make(chan RequetChannel, config.Get("server", "channels").Int(10000)),
	}
}

func (this *Server) incReq() int64 {
	return atomic.AddInt64(&this.reqs, 1)
}

func (this *Server) decReq() int64 {
	return atomic.AddInt64(&this.reqs, -1)
}

func (this *Server) getReq() int64 {
	return atomic.AddInt64(&this.reqs, 0)
}

func (this *Server) incConn() int64 {
	return atomic.AddInt64(&this.conns, 1)
}

func (this *Server) decConn() int64 {
	return atomic.AddInt64(&this.conns, -1)
}

func (this *Server) getConn() int64 {
	return atomic.AddInt64(&this.conns, 0)
}

func (s *Server) listen(ctx context.Context, req *transport.Request) error {
	ip, port, err := net.SplitHostPort(req.Addr().String())
	if err != nil {
		return err
	}

	ip, err = common.GetEthIp()
	if nil != err {
		return err
	}

	s.addrs = fmt.Sprintf("%s:%s", ip, port)
	s.registry.Register(s.addrs, "rpc")
	metrics.Host = s.addrs

	zzlog.Infow("Server.listen called", zap.String("addr", s.addrs))
	return nil
}

func (this *Server) connect(ctx context.Context, req *transport.Request) error {
	conns := this.incConn()
	metrics.CounterByAdd("server", "connect", conns)

	zzlog.Infow("Server.connect called", zap.String("from", req.RemoteAddr().String()))

	return nil
}

func (this *Server) closed(ctx context.Context, req *transport.Request) error {
	this.decConn()
	metrics.Counter("server", "close")

	zzlog.Infow("Server.closed called", zap.String("from", req.RemoteAddr().String()))

	return nil
}

// method_num|data
func (this *Server) handle(ctx context.Context, request *transport.Request, response *transport.Response) error {
	var traceId string
	defer func() {
		metrics.Counter("server", "recv")
		if r := recover(); r != nil {
			metrics.Counter("server", "panic")

			zzlog.Errorw("Server.recv error", zap.String("traceId", traceId), zap.Error(r.(error)))
		}
	}()

	msg := &proto.Request{}
	err := msg.Unmarshal(request.Packet())
	if nil != err {
		return err
	}
	traceId = msg.Headers["traceId"]
	zzlog.Debugw("Server.handle Unmarshal", zap.String("cost",
		fmt.Sprintf("%dms", time.Now().UnixMilli()-request.Stamp())))

	if _, ok := this.rpcHandler.calls[uint64(msg.GetRpcId())]; !ok {
		return errors.New(fmt.Sprintf("RpcId called not register! rid:%d traceId:%s ", msg.GetRpcId(), traceId))
	}

	item := this.rpcHandler.calls[uint64(msg.GetRpcId())]
	if nil == item || nil == item.Call {
		metrics.Counter("server", "not.Call")

		return errors.New(fmt.Sprintf("call func not exists! rid:%d	traceId:%s", msg.GetRpcId(), traceId))
	}

	reqCount := this.incReq()
	ctx = context.WithValue(ctx, "reqCount", reqCount)

	res := &proto.Response{
		Sid:     msg.Sid,
		Headers: msg.Headers,
		Code:    0,
	}

	defer func() {
		reqCount = this.decReq()
		zzlog.Debugw("Recv from client",
			zap.Int64("Sid", msg.Sid),
			zap.String("method", item.Name),
			zap.Int64("reqCount", reqCount),
			zap.Int64("conns", this.conns),
			zap.String("traceId", traceId),
			zap.String("cost", fmt.Sprintf("%dms", time.Now().UnixMilli()-request.Stamp())))

		metrics.MethodCode(item.Name, fmt.Sprintf("%d", res.Code))
		metrics.CounterByAdd("server", "reqCount", reqCount)
		metrics.Summary(item.Name, request.Stamp())
	}()

	err = this.registry.Limiter(ctx, item.Name)
	if nil != err {
		res.Code = 405
		this.reply(response, res)

		return errors.New(fmt.Sprintf("registry.Limiter error[%s]	traceId:%s", err.Error(), traceId))
	}

	cctx := context.Background()
	cctx = context.WithValue(cctx, "traceId", traceId)
	ret, err := item.Call(cctx, msg.Packet)
	if nil != err {
		res.Code = 505
		this.reply(response, res)

		return errors.New(fmt.Sprintf("recv.Call error[%s]	traceId:%s", err.Error(), traceId))
	}
	res.Packet = ret

	// only call return
	if _, ok := msg.Headers["onlyCall"]; ok || 0 == msg.Sid {
		zzlog.Debugw("Recv request from onlyCall", zap.String("traceId", traceId),
			zap.Any("Sid", msg.Sid), zap.Any("method", item.Name))

		return nil
	}

	return this.reply(response, res)
}

func (s *Server) reply(response *transport.Response, packet *proto.Response) (err error) {
	res, err := packet.Marshal()
	if nil != err {
		return err
	}

	response.Write(res)
	return nil
}

func (this *Server) onRecv(ctx context.Context, req *transport.Request, res *transport.Response) error {
	zzlog.Debugw("onRecv request from onlyCall, Will request push channel.")
	if (config.Get("server", "channels").Int(10000) - 10) < len(this.reqCh) {
		zzlog.Errorw("onRecv request channel is fully, please wait.")
		metrics.Counter("server", "channels_fully")

		msg := &proto.Request{}
		err := msg.Unmarshal(req.Packet())
		if nil != err {
			return err
		}
		body := &proto.Response{
			Sid:     msg.Sid,
			Headers: msg.Headers,
			Code:    500,
		}

		return this.reply(res, body)
	}

	this.reqCh <- RequetChannel{
		Ctx: ctx,
		Req: req,
		Res: res,
	}

	return nil
}

func (this *Server) goRecv() {
	defer func() {
		zzlog.Infow("Server.goRecv defer", zap.Any("reqCh.size", len(this.reqCh)))
	}()

	for {
		select {
		case <-this.ctx.Done():
			return

		case req := <-this.reqCh:
			err := this.handle(req.Ctx, req.Req, req.Res)
			if nil != err {
				zzlog.Errorw("Server.goRecv request handle error", zap.Any("ctx", req.Ctx), zap.Error(err.(error)))
			}
		}
	}
}

func (s *Server) NewHandler(handler *rpcHandler) {
	s.rpcHandler = handler

	return
}

func (s *Server) Release() {
	s.registry.Destroy()
	if nil != s.sock {
		s.sock.Close()
	}

	if nil != s.cancelFunc {
		s.cancelFunc()
	}
}

func (s *Server) Run(opts ...HandlerOption) {
	config := &options{
		bind: "0.0.0.0",
		port: 0,
	}

	for _, o := range opts {
		o(config)
	}

	event := transport.TransEvent{
		Listen:  s.listen,
		Connect: s.connect,
		Closed:  s.closed,
		OnRecv:  s.onRecv,
	}
	btl, err := transport.NewListener(
		transport.MaxMessageSize(1<<20),
		transport.EnableLogging(true),
		transport.Address(buffstreams.FormatAddress(config.bind, strconv.Itoa(config.port))),
		transport.Event(event),
		transport.Ctx(s.ctx),
	)
	if err != nil {
		zzlog.Errorw("ListenTCP error", zap.Error(err))

		return
	}
	s.sock = btl

	err = btl.StartAsync()
	if nil != err {
		zzlog.Errorw("StartListening error", zap.Error(err))

		return
	}

	for i := 0; i < s.coroutines; i++ {
		go s.goRecv()
	}
	go func() {
		timer := time.NewTicker(500 * time.Millisecond)

		for {
			select {
			case <-s.ctx.Done():
				return

			case <-timer.C:
				metrics.CounterByAdd("server", "channels", int64(len(s.reqCh)))
				metrics.CounterByAdd("server", "coroutines", int64(runtime.NumGoroutine()))
			}
		}
	}()
}
