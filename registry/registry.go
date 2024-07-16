package registry

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	zconfig "github.com/shockerjue/gffg/config"
	"github.com/shockerjue/gffg/zzlog"

	polaris "github.com/polarismesh/polaris-go"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/plugin/metrics/prometheus"

	"go.uber.org/zap"
)

type registry struct {
	n        *node
	addr     string
	consumer polaris.ConsumerAPI
	provider polaris.ProviderAPI
	limiter  polaris.LimitAPI
}

func Registry() *registry {
	v := &registry{}
	return v
}

func (r *registry) Provider(node *node) {
	addrs := zconfig.Get("polaris", "addrs").String("")
	if 0 == len(addrs) {
		zzlog.Fatal("registry.Provider addrs is empty!")
	}

	cfg := config.NewDefaultConfiguration(strings.Split(addrs, ","))
	if zconfig.Get("polaris", "reporter", "enable").Bool() {
		cfg.GetGlobal().GetStatReporter().SetEnable(true)
		cfg.GetGlobal().GetStatReporter().SetChain([]string{"prometheus"})

		cfg.GetGlobal().GetStatReporter().SetPluginConfig("prometheus", &prometheus.Config{
			Type:     zconfig.Get("polaris", "reporter", "prometheus", "type").String("pull"),
			Interval: time.Duration(zconfig.Get("polaris", "reporter", "prometheus", "interval").Int64(5)) * time.Second,
			Address:  zconfig.Get("polaris", "reporter", "prometheus", "address").String("127.0.0.1:9091"),
		})
	}

	provider, err := polaris.NewProviderAPIByConfig(cfg)
	if nil != err {
		zzlog.Fatalw("NewProviderAPIByConfig error", zap.Any("addrs", addrs), zap.Error(err))

		return
	}
	r.provider = provider
	r.n = node

	limit := polaris.NewLimitAPIByContext(provider.SDKContext())
	r.limiter = limit
}

func (r *registry) Consumer() {
	addrs := zconfig.Get("polaris", "addrs").String("")
	if 0 == len(addrs) {
		zzlog.Fatal("registry.Consumer addrs is empty!")
	}

	cfg := config.NewDefaultConfiguration(strings.Split(addrs, ","))
	if zconfig.Get("polaris", "reporter", "enable").Bool() {
		cfg.GetGlobal().GetStatReporter().SetEnable(true)
		cfg.GetGlobal().GetStatReporter().SetChain([]string{"prometheus"})

		cfg.GetGlobal().GetStatReporter().SetPluginConfig("prometheus", &prometheus.Config{
			Type:     zconfig.Get("polaris", "reporter", "prometheus", "type").String("pull"),
			Interval: time.Duration(zconfig.Get("polaris", "reporter", "prometheus", "interval").Int64(5)) * time.Second,
			Address:  zconfig.Get("polaris", "reporter", "prometheus", "address").String("127.0.0.1:9091"),
		})
	}

	consumer, err := polaris.NewConsumerAPIByConfig(cfg)
	if nil != err {
		zzlog.Fatalw("NewConsumerAPIByConfig error", zap.Any("addrs", addrs), zap.Error(err))

		return
	}
	r.consumer = consumer
}

func (r *registry) Register(addr string, protocl string) {
	if nil == r.provider {
		zzlog.Fatal("registry.Register provider didn't initialize!")

		return
	}

	r.addr = addr
	registerRequest := &polaris.InstanceRegisterRequest{}
	registerRequest.Service = r.n.opts.name
	registerRequest.Namespace = r.n.opts.group

	registerRequest.Version = &r.n.opts.version
	registerRequest.Protocol = &protocl

	registerRequest.Location = &model.Location{
		Region: r.n.opts.location.region,
		Zone:   r.n.opts.location.zone,
		Campus: r.n.opts.location.campus,
	}

	addrs := strings.Split(addr, ":")
	registerRequest.Host = addrs[0]
	registerRequest.Port, _ = strconv.Atoi(addrs[1])
	registerRequest.ServiceToken = r.n.opts.token
	registerRequest.SetTTL(1)
	_, err := r.provider.RegisterInstance(registerRequest)
	if nil != err {
		zzlog.Fatalw("Server register fail ", zap.Any("addr", addr))
	}
}

func (r *registry) deregister(addr string) {
	if nil == r.provider {
		zzlog.Fatal("registry.deregister provider didn't initialize!")

		return
	}

	deregisterRequest := &polaris.InstanceDeRegisterRequest{}
	deregisterRequest.Service = r.n.opts.name
	deregisterRequest.Namespace = r.n.opts.group

	addrs := strings.Split(addr, ":")
	deregisterRequest.Host = addrs[0]
	deregisterRequest.Port, _ = strconv.Atoi(addrs[1])
	deregisterRequest.ServiceToken = r.n.opts.token

	r.provider.Deregister(deregisterRequest)
}

func (r *registry) Destroy() {
	r.deregister(r.addr)
	if nil != r.provider {
		r.provider.Destroy()
	}

	if nil != r.consumer {
		r.consumer.Destroy()
	}

	if nil != r.limiter {
		r.limiter.Destroy()
	}
}

func (r *registry) Limiter(ctx context.Context, call string) error {
	quotaReq := polaris.NewQuotaRequest().(*model.QuotaRequestImpl)
	quotaReq.SetMethod(call)
	quotaReq.SetNamespace(r.n.opts.group)
	quotaReq.SetService(r.n.opts.name)

	resp, err := r.limiter.GetQuota(quotaReq)
	if err != nil {
		return err
	}

	if err != nil {
		return err
	}

	// quotaReq.GetNamespace(), quotaReq.GetService(), quotaReq.GetMethod(), quotaReq.GetLabels()
	zzlog.Debugw("registry.Limiter", zap.Any("call", call),
		zap.Any("namespace", quotaReq.GetNamespace()), zap.Any("service", quotaReq.GetService()),
		zap.Any("method", quotaReq.GetMethod()), zap.Any("labels", quotaReq.GetLabels()),
		zap.Any("resp", resp), zap.Any("model.QuotaResultOk", model.QuotaResultOk))
	if resp.Get().Code != model.QuotaResultOk {
		err = errors.New(fmt.Sprintf("%s Request too many times, already limiter request!", call))

		return err
	}

	return nil
}

func (r *registry) GetNode(ctx context.Context, group, name string) (instance NodeInstance, err error) {
	if nil == r.consumer {
		err = errors.New("GetNode consumer is nil, didn't initialize!")

		return
	}

	getOneRequest := &polaris.GetOneInstanceRequest{}
	getOneRequest.Namespace = group
	getOneRequest.Service = name
	oneInstResp, err := r.consumer.GetOneInstance(getOneRequest)
	if nil != err {
		return
	}

	ins := oneInstResp.GetInstance()
	if nil == ins {
		err = errors.New(fmt.Sprintf("GetNode fail for %s:%s", group, name))

		return
	}

	instance = NodeInstance(ins)
	return
}
