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
	MaxCh   = 10000
	MaxPush = 1000
)

type metrics struct {
	pub *kafka.Product
	mCh chan *proto.Metric
}

var _m *metrics
var once sync.Once

func obj() *metrics {
	once.Do(func() {
		_m = &metrics{
			pub: kafka.NewProduct(
				kafka.Brokers(config.Get("metrics", "brokers").String("")),
				kafka.Group(config.Get("metrics", "group").String("")),
				kafka.Topic(config.Get("metrics", "topic").String(""))),
			mCh: make(chan *proto.Metric, MaxCh),
		}

		go _m.loop()
	})

	return _m
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

	for {
		lists := make([]*proto.Metric, 0)
		for {
			select {
			case it := <-m.mCh:
				lists = append(lists, it)

			case <-timer.C:
				goto to
			}
		}

	to:
		m.combine(lists)
	}
}

func (m *metrics) report(it *proto.Metrics) {
	if nil == it {
		return
	}

	startAt := time.Now().UnixMilli()
	buffer, err := it.Marshal()
	if nil != err {
		zzlog.Errorw("metrics.send msg.Marshal error", zap.Error(err),
			zap.Any("cost", time.Now().UnixMilli()-startAt), zap.Any("mCh", len(m.mCh)))

		return
	}

	err = m.pub.Product(context.TODO(), buffer)
	if nil != err {
		zzlog.Errorw("metrics.send Product error", zap.Error(err),
			zap.Any("cost", time.Now().UnixMilli()-startAt), zap.Any("mCh", len(m.mCh)))

		return
	}

	zzlog.Debugw("metrics.send success", zap.Any("buffer.size", len(buffer)),
		zap.Any("cost", time.Now().UnixMilli()-startAt), zap.Any("mCh", len(m.mCh)))
	return
}

func (m *metrics) to(it *proto.Metric) {
	if (MaxCh - 10) < len(m.mCh) {
		zzlog.Warnw("metrics.to channel is fully", zap.Any("mCh", len(m.mCh)))

		return
	}

	m.mCh <- it
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
