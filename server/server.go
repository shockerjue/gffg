package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shockerjue/gffg/circuitbreaker"
	"github.com/shockerjue/gffg/common"
	"github.com/shockerjue/gffg/config"
	"github.com/shockerjue/gffg/metrics"
	"github.com/shockerjue/gffg/proto"
	"github.com/shockerjue/gffg/ratelimit"
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

	reqCh          chan RequetChannel
	rateLimiter    ratelimit.Limiter
	circuitBreaker circuitbreaker.CircuitBreaker
	rateLimitMgr   *ratelimit.Manager
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

	server := &Server{
		ctx:        ctx,
		cancelFunc: cFunc,
		registry:   opt.registry,
		coroutines: config.Get("server", "coroutines").Int(32),
		reqCh:      make(chan RequetChannel, config.Get("server", "channels").Int(10000)),
	}

	// Initialize rate limiter if enabled
	if config.Get("server", "rate_limit", "enabled").Bool() {
		server.initRateLimiter()
	}

	// Initialize circuit breaker if enabled
	if config.Get("server", "circuit_breaker", "enabled").Bool() {
		server.initCircuitBreaker()
	}

	// Initialize rate limit manager
	server.initRateLimitManager()

	return server
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

			var err error
			switch v := r.(type) {
			case error:
				err = v
			case string:
				err = errors.New(v)
			default:
				err = fmt.Errorf("panic recovered: %v", v)
			}
			zzlog.Errorw("Server.recv error", zap.String("traceId", traceId), zap.Error(err))
		}
	}()

	msg := &proto.Request{}
	err := msg.Unmarshal(request.Packet())
	if nil != err {
		return common.ErrRPCUnmarshal(traceId, err)
	}
	traceId = msg.Headers["traceId"]
	if zzlog.IsDebugEnabled() {
		zzlog.Debugw("Server.handle Unmarshal", zap.String("cost",
			fmt.Sprintf("%dms", time.Now().UnixMilli()-request.Stamp())))
	}

	if _, ok := this.rpcHandler.calls[uint64(msg.GetRpcId())]; !ok {
		return common.ErrRPCNotRegistered(uint64(msg.GetRpcId()), traceId)
	}

	item := this.rpcHandler.calls[uint64(msg.GetRpcId())]
	if nil == item || nil == item.Call {
		metrics.Counter("server", "not.Call")

		return common.ErrRPCNotFound(uint64(msg.GetRpcId()), traceId)
	}

	// Apply rate limiting if enabled
	if this.rateLimiter != nil {
		allowed, rlErr := this.rateLimiter.Allow(ctx, item.Name)
		if rlErr != nil {
			zzlog.Warnw("rate limiter error", zap.String("method", item.Name), zap.Error(rlErr))
		} else if !allowed {
			res := &proto.Response{
				Sid:     msg.Sid,
				Headers: msg.Headers,
				Code:    common.ErrCodeRateLimit,
			}
			this.reply(response, res)
			return common.ErrRPCRateLimit(item.Name, traceId)
		}
	}

	// Apply circuit breaker if enabled
	if this.circuitBreaker != nil {
		if !this.circuitBreaker.AllowRequest(ctx) {
			res := &proto.Response{
				Sid:     msg.Sid,
				Headers: msg.Headers,
				Code:    common.ErrCodeCallFailed,
			}
			this.reply(response, res)
			return common.NewRPCError(503, "service unavailable due to circuit breaker", traceId)
		}
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
		cost := time.Now().UnixMilli() - request.Stamp()

		// Only log debug info if enabled or if there's an error
		if zzlog.IsDebugEnabled() || res.Code != 0 || cost > 1000 {
			zzlog.Debugw("Recv from client",
				zap.Int64("Sid", msg.Sid),
				zap.String("method", item.Name),
				zap.Int64("reqCount", reqCount),
				zap.Int64("conns", this.conns),
				zap.String("traceId", traceId),
				zap.Int64("cost_ms", cost),
				zap.Int32("response_code", res.Code))
		}

		metrics.MethodCode(item.Name, fmt.Sprintf("%d", res.Code))
		metrics.CounterByAdd("server", "reqCount", reqCount)
		metrics.Summary(item.Name, request.Stamp())

		// Record result for circuit breaker
		if this.circuitBreaker != nil {
			if res.Code != 0 {
				this.circuitBreaker.RecordFailure(fmt.Errorf("RPC failed with code %d", res.Code))
			} else {
				this.circuitBreaker.RecordSuccess()
			}
		}
	}()

	// Apply registry limiter (external service)
	err = this.registry.Limiter(ctx, item.Name)
	if nil != err {
		res.Code = common.ErrCodeRateLimit
		this.reply(response, res)

		return common.ErrRPCRateLimit(item.Name, traceId)
	}

	cctx := context.Background()
	cctx = context.WithValue(cctx, "traceId", traceId)
	ret, err := item.Call(cctx, msg.Packet)
	if nil != err {
		res.Code = common.ErrCodeCallFailed
		this.reply(response, res)

		return common.ErrRPCCallFailed(item.Name, traceId, err)
	}
	res.Packet = ret

	// only call return
	if _, ok := msg.Headers["onlyCall"]; ok || 0 == msg.Sid {
		if zzlog.IsDebugEnabled() {
			zzlog.Debugw("Recv request from onlyCall", zap.String("traceId", traceId),
				zap.Int64("Sid", msg.Sid), zap.String("method", item.Name))
		}

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
	if zzlog.IsDebugEnabled() {
		zzlog.Debugw("onRecv request, pushing to channel.")
	}

	if (config.Get("server", "channels").Int(10000) - 10) < len(this.reqCh) {
		// Only log error occasionally to avoid log spam
		if time.Now().Unix()%5 == 0 { // Log once every 5 seconds
			zzlog.Errorw("onRecv request channel is fully, please wait.",
				zap.Int("channel_size", len(this.reqCh)),
				zap.Int("channel_capacity", config.Get("server", "channels").Int(10000)))
		}
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
		if zzlog.IsDebugEnabled() {
			zzlog.Debugw("Server.goRecv exiting", zap.Int("reqCh_size", len(this.reqCh)))
		}
	}()

	for {
		select {
		case <-this.ctx.Done():
			return

		case req := <-this.reqCh:
			err := this.handle(req.Ctx, req.Req, req.Res)
			if nil != err {
				// Log connection errors at warn level, other errors at error level
				if strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "broken pipe") {
					zzlog.Warnw("Server.goRecv connection error", zap.Error(err))
				} else {
					zzlog.Errorw("Server.goRecv request handle error", zap.Error(err))
				}
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

	// Close rate limiter if available
	if s.rateLimiter != nil {
		s.rateLimiter.Close()
	}

	// Close circuit breaker if available
	if s.circuitBreaker != nil {
		s.circuitBreaker.Close()
	}

	// Close rate limit manager if available
	if s.rateLimitMgr != nil {
		s.rateLimitMgr.Close()
	}
}

func (s *Server) Run(opts ...HandlerOption) {
	optsConfig := &options{
		bind: "0.0.0.0",
		port: 0,
	}

	for _, o := range opts {
		o(optsConfig)
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
		transport.Address(buffstreams.FormatAddress(optsConfig.bind, strconv.Itoa(optsConfig.port))),
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

	// Start metrics collection for rate limiting and circuit breaking
	go s.collectMetrics()

	go func() {
		timer := time.NewTicker(5 * time.Second) // Reduced frequency from 500ms to 5s

		for {
			select {
			case <-s.ctx.Done():
				return

			case <-timer.C:
				metrics.CounterByAdd("server", "channels", int64(len(s.reqCh)))
				metrics.CounterByAdd("server", "coroutines", int64(runtime.NumGoroutine()))

				// Log channel status only if debug is enabled or if channel is nearly full
				if zzlog.IsDebugEnabled() || len(s.reqCh) > config.Get("server", "channels").Int(10000)*8/10 {
					zzlog.Debugw("Server status",
						zap.Int("channel_size", len(s.reqCh)),
						zap.Int("channel_capacity", config.Get("server", "channels").Int(10000)),
						zap.Int("goroutines", runtime.NumGoroutine()))
				}
			}
		}
	}()
}

// initRateLimiter initializes the rate limiter for the server
func (s *Server) initRateLimiter() {
	rateLimitConfig := &ratelimit.Config{
		DefaultLimit: ratelimit.Limit{
			Rate:      float64(config.Get("server", "rate_limit", "global").Int(1000)),
			Burst:     config.Get("server", "rate_limit", "burst").Int(2000),
			Period:    time.Second,
			Algorithm: ratelimit.TokenBucket,
		},
		StorageBackend: ratelimit.LocalStorage,
		LocalConfig: &ratelimit.LocalConfig{
			MaxKeys:         config.Get("server", "rate_limit", "max_keys").Int(10000),
			CleanupInterval: 5 * time.Minute,
		},
		CleanupInterval: 5 * time.Minute,
		MetricsEnabled:  true,
		DynamicLimits:   config.Get("server", "rate_limit", "dynamic").Bool(),
	}

	limiter, err := ratelimit.NewTokenBucket(rateLimitConfig)
	if err != nil {
		zzlog.Errorw("failed to initialize rate limiter", zap.Error(err))
		return
	}

	s.rateLimiter = limiter
	zzlog.Infow("rate limiter initialized",
		zap.Float64("rate", rateLimitConfig.DefaultLimit.Rate),
		zap.Int("burst", rateLimitConfig.DefaultLimit.Burst))
}

// initCircuitBreaker initializes the circuit breaker for the server
func (s *Server) initCircuitBreaker() {
	cbConfig := circuitbreaker.Config{
		Name:                config.Get("server", "name").String("") + "-circuit-breaker",
		ErrorThreshold:      config.Get("server", "circuit_breaker", "error_threshold").Float64(0.5),
		RequestThreshold:    config.Get("server", "circuit_breaker", "request_threshold").Int64(20),
		SleepWindow:         time.Duration(config.Get("server", "circuit_breaker", "sleep_window").Int(5)) * time.Second,
		HalfOpenMaxRequests: config.Get("server", "circuit_breaker", "half_open_max_requests").Int64(5),
		Timeout:             time.Duration(config.Get("server", "circuit_breaker", "timeout").Int(30)) * time.Second,
		RollingWindow:       time.Duration(config.Get("server", "circuit_breaker", "rolling_window").Int(10)) * time.Second,
		BucketCount:         config.Get("server", "circuit_breaker", "bucket_count").Int(10),
		FailurePredicate:    circuitbreaker.DefaultFailurePredicate,
		OnStateChange: func(from, to circuitbreaker.State) {
			zzlog.Warnw("circuit breaker state changed",
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
		MetricsEnabled: true,
	}

	cb, err := circuitbreaker.NewCircuitBreaker(cbConfig)
	if err != nil {
		zzlog.Errorw("failed to initialize circuit breaker", zap.Error(err))
		return
	}

	s.circuitBreaker = cb
	zzlog.Infow("circuit breaker initialized",
		zap.String("name", cbConfig.Name),
		zap.Float64("error_threshold", cbConfig.ErrorThreshold))
}

// initRateLimitManager initializes the rate limit manager
func (s *Server) initRateLimitManager() {
	mgrConfig := &ratelimit.ManagerConfig{
		DefaultRateLimit: ratelimit.Limit{
			Rate:      float64(config.Get("server", "rate_limit", "global").Int(1000)),
			Burst:     config.Get("server", "rate_limit", "burst").Int(2000),
			Period:    time.Second,
			Algorithm: ratelimit.TokenBucket,
		},
		DefaultCircuitBreaker: circuitbreaker.DefaultConfig("default"),
		StorageBackend:        ratelimit.LocalStorage,
		LocalConfig: &ratelimit.LocalConfig{
			MaxKeys:         config.Get("server", "rate_limit", "max_keys").Int(10000),
			CleanupInterval: 5 * time.Minute,
		},
		CleanupInterval:    5 * time.Minute,
		MetricsEnabled:     true,
		DynamicLimits:      config.Get("server", "rate_limit", "dynamic").Bool(),
		MaxLimiters:        config.Get("server", "rate_limit", "max_limiters").Int(1000),
		MaxCircuitBreakers: config.Get("server", "circuit_breaker", "max_circuit_breakers").Int(1000),
	}

	mgr, err := ratelimit.NewManager(mgrConfig)
	if err != nil {
		zzlog.Errorw("failed to initialize rate limit manager", zap.Error(err))
		return
	}

	s.rateLimitMgr = mgr
	zzlog.Info("rate limit manager initialized")
}

// collectMetrics collects metrics for rate limiting and circuit breaking
func (s *Server) collectMetrics() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.rateLimitMgr != nil {
				metrics := s.rateLimitMgr.GetMetrics()
				// Report metrics to monitoring system
				zzlog.Debugw("rate limit manager metrics",
					zap.Int64("rate_limit_allowed", metrics.RateLimitAllowed),
					zap.Int64("rate_limit_denied", metrics.RateLimitDenied),
					zap.Int64("circuit_breaker_requests", metrics.CircuitBreakerRequests),
					zap.Int64("circuit_breaker_success", metrics.CircuitBreakerSuccess),
					zap.Int64("circuit_breaker_failures", metrics.CircuitBreakerFailures),
					zap.Int64("circuit_breaker_rejected", metrics.CircuitBreakerRejected))

				metrics.CounterByAdd("server", "rate_limit_allowed", metrics.rate_limit_allowed)
				metrics.CounterByAdd("server", "rate_limit_denied", metrics.rate_limit_denied)
				metrics.CounterByAdd("server", "circuit_breaker_requests", metrics.circuit_breaker_requests)
				metrics.CounterByAdd("server", "circuit_breaker_success", metrics.circuit_breaker_success)
				metrics.CounterByAdd("server", "circuit_breaker_failures", metrics.circuit_breaker_failures)
				metrics.CounterByAdd("server", "circuit_breaker_rejected", metrics.circuit_breaker_rejected)
			}
		}
	}
}

// SetRateLimit sets or updates the rate limit for a specific method
func (s *Server) SetRateLimit(method string, limit ratelimit.Limit) error {
	if s.rateLimitMgr == nil {
		return fmt.Errorf("rate limit manager not initialized")
	}

	serviceName := config.Get("server", "name").String("")
	return s.rateLimitMgr.SetRateLimit(context.Background(), serviceName, method, limit)
}

// SetCircuitBreaker sets or updates the circuit breaker for a specific method
func (s *Server) SetCircuitBreaker(method string, cbConfig circuitbreaker.Config) error {
	if s.rateLimitMgr == nil {
		return fmt.Errorf("rate limit manager not initialized")
	}

	serviceName := config.Get("server", "name").String("")
	return s.rateLimitMgr.SetCircuitBreaker(serviceName, method, cbConfig)
}
