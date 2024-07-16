package transport

import "context"

type TransOption func(*options)

type options struct {
	maxMessageSize int32
	headerByteSize int32
	enableLogging  bool
	address        string
	event          TransEvent
	ctx            context.Context
}

func MaxMessageSize(maxMessageSize int32) TransOption {
	return func(c *options) {
		c.maxMessageSize = maxMessageSize
	}
}

func HeaderByteSize(headerByteSize int32) TransOption {
	return func(c *options) {
		c.headerByteSize = headerByteSize
	}
}

func EnableLogging(enableLogging bool) TransOption {
	return func(c *options) {
		c.enableLogging = enableLogging
	}
}

func Address(address string) TransOption {
	return func(c *options) {
		c.address = address
	}
}

func Event(event TransEvent) TransOption {
	return func(c *options) {
		c.event = event
	}
}

func Ctx(ctx context.Context) TransOption {
	return func(c *options) {
		c.ctx = ctx
	}
}

func initOpts(opts ...TransOption) *options {
	var opt options
	for _, o := range opts {
		o(&opt)
	}

	return &opt
}
