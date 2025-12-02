package client

import (
	"context"
	"time"

	"github.com/shockerjue/gffg/circuitbreaker"
	"github.com/shockerjue/gffg/ratelimit"
	"github.com/shockerjue/gffg/registry"
)

type CallOption func(*Options)
type ClientOption func(*Options)

type Options struct {
	onlyCall bool
	timeout  int32

	ctx context.Context
	// client option
	registry        registry.IRegistry
	circuitBreaker  circuitbreaker.CircuitBreaker
	rateLimiter     ratelimit.Limiter
	rateLimitConfig *ratelimit.Limit
	cbConfig        *circuitbreaker.Config
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

// CircuitBreaker sets a custom circuit breaker for the client
func CircuitBreaker(cb circuitbreaker.CircuitBreaker) ClientOption {
	return func(args *Options) {
		args.circuitBreaker = cb
	}
}

// RateLimiter sets a custom rate limiter for the client
func RateLimiter(rl ratelimit.Limiter) ClientOption {
	return func(args *Options) {
		args.rateLimiter = rl
	}
}

// RateLimitConfig sets the rate limit configuration for the client
func RateLimitConfig(limit ratelimit.Limit) ClientOption {
	return func(args *Options) {
		args.rateLimitConfig = &limit
	}
}

// CircuitBreakerConfig sets the circuit breaker configuration for the client
func CircuitBreakerConfig(config circuitbreaker.Config) ClientOption {
	return func(args *Options) {
		args.cbConfig = &config
	}
}

// DefaultRateLimit returns a sensible default rate limit configuration
func DefaultRateLimit() ratelimit.Limit {
	return ratelimit.Limit{
		Rate:      1000,
		Burst:     2000,
		Period:    time.Second,
		Algorithm: ratelimit.TokenBucket,
	}
}

// DefaultCircuitBreakerConfig returns a sensible default circuit breaker configuration
func DefaultCircuitBreakerConfig(name string) circuitbreaker.Config {
	return circuitbreaker.DefaultConfig(name)
}
