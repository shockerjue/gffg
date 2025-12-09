package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/shockerjue/gffg/config"
	"github.com/shockerjue/gffg/kafka"
	"github.com/shockerjue/gffg/proto"
	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

var (
	Host    = ""
	MaxCh   = 100000
	MaxPush = 2000

	// Internal metrics for monitoring the metrics system itself
	metricsDropped = &counter{}
)

type counter struct {
	sync.RWMutex
	value int64
}

func (c *counter) Inc() {
	c.Lock()
	defer c.Unlock()
	c.value++
}

func (c *counter) Get() int64 {
	c.RLock()
	defer c.RUnlock()
	return c.value
}

type metrics struct {
	pub    *kafka.Product
	mCh    chan *proto.Metric
	ctx    context.Context
	cancel context.CancelFunc
}

var _m *metrics
var once sync.Once

func obj() *metrics {
	once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		_m = &metrics{
			pub: kafka.NewProduct(
				kafka.Brokers(config.Get("metrics", "brokers").String("")),
				kafka.Group(config.Get("metrics", "group").String("")),
				kafka.Topic(config.Get("metrics", "topic").String(""))),
			mCh:    make(chan *proto.Metric, MaxCh),
			ctx:    ctx,
			cancel: cancel,
		}

		go _m.loop()

		// Start background goroutine to monitor metrics system health
		go _m.monitor()
	})

	return _m
}

func (m *metrics) monitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			dropped := metricsDropped.Get()
			if dropped > 0 {
				zzlog.Warnw("metrics system health check",
					zap.Int64("metrics_dropped", dropped),
					zap.Int("channel_size", len(m.mCh)),
					zap.Int("channel_capacity", MaxCh))
			}
		}
	}
}

func (m *metrics) combine(its []*proto.Metric) {
	if nil == its || 0 == len(its) {
		return
	}

	var mBatch proto.Metrics
	for its != nil {
		if MaxPush < len(its) {
			mBatch.Lists = its[:MaxPush]
			its = its[MaxPush:]
		} else {
			mBatch.Lists = its
			its = nil
		}

		m.report(&mBatch)
	}
}

func (m *metrics) loop() {
	timer := time.NewTicker(200 * time.Millisecond)
	batchSize := MaxPush // Pre-allocate reasonable batch size

	lists := make([]*proto.Metric, 0, batchSize)
	for {

		// Wait for first metric or timer
		select {
		case it := <-m.mCh:
			lists = append(lists, it)

		case <-timer.C:
			// Timer expired, process any collected metrics
			if len(lists) > 0 {
				m.combine(lists)

				lists = lists[:0]
			}

		}
	}
}

func (m *metrics) report(it *proto.Metrics) {
	if nil == it || len(it.Lists) == 0 {
		return
	}

	startAt := time.Now().UnixMilli()
	buffer, err := it.Marshal()
	if nil != err {
		// Use Error level only for critical errors
		zzlog.Warnw("metrics.send msg.Marshal error", zap.Error(err),
			zap.Any("cost", time.Now().UnixMilli()-startAt), zap.Any("mCh", len(m.mCh)),
			zap.Int("batch_size", len(it.Lists)))

		return
	}

	err = m.pub.Product(context.TODO(), buffer)
	if nil != err {
		zzlog.Warnw("metrics.send Product error", zap.Error(err),
			zap.Any("cost", time.Now().UnixMilli()-startAt), zap.Any("mCh", len(m.mCh)),
			zap.Int("batch_size", len(it.Lists)))

		return
	}

	// Only log debug info if enabled
	if zzlog.IsDebugEnabled() {
		zzlog.Debugw("metrics.send success", zap.Any("buffer.size", len(buffer)),
			zap.Any("cost", time.Now().UnixMilli()-startAt), zap.Any("mCh", len(m.mCh)),
			zap.Int("batch_size", len(it.Lists)))
	}
}

func (m *metrics) to(it *proto.Metric) {
	if (MaxCh - 10) < len(m.mCh) {
		// Only log warning occasionally to avoid log spam
		if time.Now().Unix()%10 == 0 { // Log once every 10 seconds
			zzlog.Warnw("metrics.to channel is fully", zap.Any("mCh", len(m.mCh)))
		}

		// Drop metric instead of blocking
		metricsDropped.Inc()
		return
	}

	select {
	case m.mCh <- it:
		// Successfully sent
	default:
		// Channel is full, drop the metric
		metricsDropped.Inc()
		if time.Now().Unix()%10 == 0 {
			zzlog.Warnw("metrics.to channel full, dropping metric", zap.Any("mCh", len(m.mCh)))
		}
	}
	return
}

// Monitoring inc
//
// @param	_type 	Monitoring Metrics
// @param	value 	Monitoring value
// @param	opts
func Counter(_type, value string, opts ...MetricOption) {
	opt := &option{}
	for _, o := range opts {
		o(opt)
	}
	if len(opt.serveName) == 0 {
		opt.serveName = config.Get("server", "name").String("")
	}

	metrc := &proto.Metric{
		Type: proto.MetricType_GaugeType,
		Gauge: &proto.Gauge{
			Type:  _type,
			Value: value,
			Inc:   true,
		},
		Host:    Host,
		Svrname: opt.serveName,
	}

	obj().to(metrc)
}

// Monitoring add value
//
// @param	_type 	Monitoring Metrics
// @param	value 	Monitoring value
// @param	add 	add  value
// @param	opts
func CounterByAdd(_type, value string, add int64, opts ...MetricOption) {
	opt := &option{}
	for _, o := range opts {
		o(opt)
	}
	if len(opt.serveName) == 0 {
		opt.serveName = config.Get("server", "name").String("")
	}

	metrc := &proto.Metric{
		Type: proto.MetricType_GaugeType,
		Gauge: &proto.Gauge{
			Type:  _type,
			Value: value,
			Add:   add,
		},
		Svrname: opt.serveName,
		Host:    Host,
	}

	obj().to(metrc)
}

// Method and code Metrics
//
// @param	method 	method Metrics
// @param	code 	code Metrics
// @param	opts
func MethodCode(method, code string, opts ...MetricOption) {
	opt := &option{}
	for _, o := range opts {
		o(opt)
	}
	if len(opt.serveName) == 0 {
		opt.serveName = config.Get("server", "name").String("")
	}

	metrc := &proto.Metric{
		Type: proto.MetricType_CounterType,
		Counter: &proto.Counter{
			Method: method,
			Code:   code,
		},
		Svrname: opt.serveName,
		Host:    Host,
	}

	obj().to(metrc)
}

// Summary  Metrics
//
// @param	method 	method  Metrics
// @param	startAt	timestamp
// @param	opts
func Summary(method string, startAt int64, opts ...MetricOption) {
	opt := &option{}
	for _, o := range opts {
		o(opt)
	}
	if len(opt.serveName) == 0 {
		opt.serveName = config.Get("server", "name").String("")
	}

	duration := time.Now().UnixMilli() - startAt
	if 0 > duration {
		duration = 0
	}
	metrc := &proto.Metric{
		Type: proto.MetricType_SummaryType,
		Summary: &proto.Summary{
			Method: method,
		},
		Micro:   duration,
		Host:    Host,
		Svrname: opt.serveName,
	}

	obj().to(metrc)
}
