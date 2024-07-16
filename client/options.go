package client

import (
	"context"

	"github.com/shockerjue/gffg/registry"
)

type CallOption func(*Options)
type ClientOption func(*Options)

type Options struct {
	onlyCall bool
	timeout  int32

	ctx context.Context
	// client option
	registry registry.IRegistry
}

// Just call, the rpc service will not respond
func OnlyCall(onlyCall bool) CallOption {
	return func(args *Options) {
		args.onlyCall = onlyCall
	}
}

// Timeout for calling RPC service request
func Timeout(timeout int32) CallOption {
	return func(args *Options) {
		args.timeout = timeout
	}
}

func SetOption(k, v interface{}) CallOption {
	return func(o *Options) {
		if o.ctx == nil {
			o.ctx = context.Background()
		}
		o.ctx = context.WithValue(o.ctx, k, v)
	}
}

func initOpt(opts ...CallOption) *Options {
	var opt Options
	for _, o := range opts {
		o(&opt)
	}
	if 0 == opt.timeout {
		opt.timeout = 3
	}

	return &opt
}

// Custom Registry for service manage
func Registry(registry registry.IRegistry) ClientOption {
	return func(args *Options) {
		args.registry = registry
	}
}
