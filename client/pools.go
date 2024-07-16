package client

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/shockerjue/gffg/common"
	"github.com/shockerjue/gffg/metrics"
	"github.com/shockerjue/gffg/proto"
	"github.com/shockerjue/gffg/registry"
	"github.com/shockerjue/gffg/transport"
	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

type pool struct {
	rw  sync.RWMutex
	wrw sync.RWMutex

	// Each RPC request item is used to
	// wait for the service response
	callItem map[int64]*CallCond

	// RPC service connection pool,
	// each service will have 8 connections
	rpcconn map[string][]*client

	r    registry.IRegistry
	opts *Options
}

func newPool(r registry.IRegistry) *pool {
	instance := &pool{
		rpcconn:  make(map[string][]*client),
		callItem: make(map[int64]*CallCond),
		r:        r,
	}
	instance.r.Consumer()
	ip, _ := common.GetEthIp()
	metrics.Host = ip

	return instance
}

func (p *pool) key(group, svrname string) string {
	return group + svrname
}

func (p *pool) destroy() {
	p.r.Destroy()

	p.rw.RLock()
	defer p.rw.RUnlock()
	for _, v := range p.rpcconn {
		for _, c := range v {
			c.S.Close()
		}
	}

	p.wrw.RLock()
	defer p.wrw.RUnlock()
	for _, v := range p.callItem {
		close(v.Ch)
	}
}

func (p *pool) connect(ctx context.Context, group, svrname, name string) (
	s *transport.Socket, instance registry.NodeInstance, err error) {
	instance, err = p.r.GetNode(ctx, group, svrname)
	if nil != err {
		zzlog.Errorw("pool.connect error", zap.Any("svrname", svrname), zap.Any("name", name), zap.Error(err))

		return
	}

	addr := fmt.Sprintf("%s:%d", instance.GetHost(), instance.GetPort())
	s, err = transport.SocketByAddr(addr)
	if nil != err {
		return
	}

	go s.Handle(func(ctx context.Context, req *transport.Request, r *transport.Response) error {
		defer func() {
			if r := recover(); r != nil {
				zzlog.Errorw("client.Handle error", zap.Error(r.(error)))

				return
			}
		}()

		msg := &proto.Response{}
		err := msg.Unmarshal(req.Packet())
		if nil != err {
			return nil
		}

		if 0 != msg.Code {
			zzlog.Warnw("Recv from server fail", zap.Int32("code", msg.Code),
				zap.Any("sid", msg.Sid), zap.Any("headers", msg.Headers))
		}

		// only call, needn't response
		Sid := msg.Sid
		if _, ok := msg.Headers["onlyCall"]; ok || 0 == Sid {
			zzlog.Errorw("Recv from server fail", zap.Any("code", msg.Code), zap.Any("sid", Sid))

			return nil
		}

		p.wrw.RLock()
		if _, ok := p.callItem[Sid]; ok {
			p.callItem[Sid].Packet = msg.Packet
			p.callItem[Sid].Code = msg.Code

			if !common.ClosedChanInt(p.callItem[Sid].Ch) {
				p.callItem[Sid].Ch <- 0
			}
		}
		p.wrw.RUnlock()

		zzlog.Debugw("Recv from server", zap.Int64("Sid", Sid), zap.Any("Header",
			msg.Headers), zap.Any("packet.len", len(msg.Packet)))
		return nil
	}, func(ctx context.Context, req *transport.Request) error {
		p.removeByClient(group, svrname, name, req.RemoteAddr().String())

		return nil
	})
	return
}

func (p *pool) response(ctx context.Context, group, svrname string) (c client, err error) {
	p.rw.Lock()
	defer p.rw.Unlock()

	// Establish a connection pool, and establish 8 connections for each service.
	// Always maintain 8 connections
	genCon := func(num int) []*client {
		rpcconn := make([]*client, 0)
		for i := 0; i < num; i++ {
			name := common.GenUid()
			skt, instance, err := p.connect(ctx, group, svrname, name)
			if nil != err {
				zzlog.Errorw("pool.connect error", zap.Error(err))

				continue
			}

			rpcconn = append(rpcconn, &client{
				S:        skt,
				Svrname:  svrname,
				Name:     name,
				Group:    group,
				instance: instance,
				Stamp:    time.Now().Unix(),
			})
		}

		return rpcconn
	}

	key := p.key(group, svrname)
	if 0 == len(p.rpcconn[key]) {
		p.rpcconn[key] = genCon(RPC_POOL_SIZE)
	}

	if len(p.rpcconn[key]) < RPC_POOL_SIZE {
		rpcconn := genCon(RPC_POOL_SIZE - len(p.rpcconn[key]))

		p.rpcconn[key] = append(p.rpcconn[key], rpcconn...)
	}

	if 0 == len(p.rpcconn[key]) {
		err = errors.New(fmt.Sprintf("%s didn't more node!", key))

		return
	}

	metrics.CounterByAdd("client", "connect", int64(len(p.rpcconn[key])))

	// choice server node with random
	seed := time.Now().UnixNano()
	src := rand.NewSource(seed)
	rng := rand.New(src)

	length := rng.Intn(len(p.rpcconn[key]))
	p.rpcconn[key][length].Stamp = time.Now().Unix()
	c = *p.rpcconn[key][length]

	zzlog.Debugw("Select client connect", zap.Int("index", length))
	return
}

func (p *pool) removeByClient(group, svrname, name, addr string) {
	index := -1
	defer func() {
		metrics.Counter("client", "remove")
		zzlog.Warnw("pool.removeByClient", zap.Any("index", index),
			zap.Any("group", group), zap.Any("svrname", svrname), zap.Any("name", name))
	}()

	rpcconn := make([]*client, 0)
	p.rw.RLock()
	key := p.key(group, svrname)
	for k, v := range p.rpcconn[key] {
		if v.Name == name {
			index = k

			continue
		}

		rpcconn = append(rpcconn, &client{
			S:        v.S,
			Svrname:  v.Svrname,
			Name:     v.Name,
			Group:    v.Group,
			Stamp:    v.Stamp,
			instance: v.instance,
		})
	}
	p.rw.RUnlock()

	if -1 != index {
		p.rw.Lock()
		p.rpcconn[key] = rpcconn
		p.rw.Unlock()
	}
}
