package server

import (
	"context"
	"time"

	"github.com/shockerjue/gffg/circuitbreaker"
	"github.com/shockerjue/gffg/ratelimit"
	"github.com/shockerjue/gffg/registry"
)

// Create server's options
type ServerOption func(*options)

type HandlerOption func(*options)

type options struct {
	bind string
	port int

	ctx context.Context
	// server option
	registry        registry.IRegistry
	rateLimiter     ratelimit.Limiter
	circuitBreaker  circuitbreaker.CircuitBreaker
	rateLimitConfig *ratelimit.Limit
	cbConfig        *circuitbreaker.Config
}

func Bind(addr string) HandlerOption {
	return func(c *options) {
		c.bind = addr
	}
}

func Port(port int) HandlerOption {
	return func(c *options) {
		c.port = port
	}
}

func Registry(registry registry.IRegistry) ServerOption {
	return func(c *options) {
		c.registry = registry
	}
}

func SetOption(k, v interface{}) HandlerOption {
	return func(o *options) {
		if o.ctx == nil {
			o.ctx = context.Background()
		}
		o.ctx = context.WithValue(o.ctx, k, v)
	}
}

// RateLimiter sets a custom rate limiter for the server
func RateLimiter(rl ratelimit.Limiter) ServerOption {
	return func(o *options) {
		o.rateLimiter = rl
	}
}

// CircuitBreaker sets a custom circuit breaker for the server
func CircuitBreaker(cb circuitbreaker.CircuitBreaker) ServerOption {
	return func(o *options) {
		o.circuitBreaker = cb
	}
}

// RateLimitConfig sets the rate limit configuration for the server
func RateLimitConfig(limit ratelimit.Limit) ServerOption {
	return func(o *options) {
		o.rateLimitConfig = &limit
	}
}

// CircuitBreakerConfig sets the circuit breaker configuration for the server
func CircuitBreakerConfig(config circuitbreaker.Config) ServerOption {
	return func(o *options) {
		o.cbConfig = &config
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
