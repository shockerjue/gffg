package server

import (
	"context"

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
	registry registry.IRegistry
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
