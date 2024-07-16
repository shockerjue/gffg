package kafka

import "context"

type Options func(*options)

type options struct {
	topic   string // Currently not in use
	group   string // The group
	brokers string // Connection address list
	ctx     context.Context
}

func Topic(topic string) Options {
	return func(c *options) {
		c.topic = topic
	}
}

func Group(group string) Options {
	return func(c *options) {
		c.group = group
	}
}

func Brokers(brokers string) Options {
	return func(c *options) {
		c.brokers = brokers
	}
}

func Ctx(ctx context.Context) Options {
	return func(c *options) {
		c.ctx = ctx
	}
}
