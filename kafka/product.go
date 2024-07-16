package kafka

import (
	"context"
	"strings"

	"github.com/IBM/sarama"
)

type Product struct {
	pub sarama.SyncProducer

	opts *options
}

// Create kafka product
//
// @param	opts 	Options
func NewProduct(opts ...Options) *Product {
	opt := &options{}
	for _, o := range opts {
		o(opt)
	}

	brokers := strings.Split(opt.brokers, ",")
	kc := sarama.NewConfig()
	kc.Producer.RequiredAcks = sarama.WaitForAll // Wait for all in-sync replicas to ack the message
	kc.Producer.Retry.Max = 10                   // Retry up to 10 times to produce the message
	kc.Producer.Return.Successes = true
	pub, err := sarama.NewSyncProducer(brokers, kc)
	if err != nil {
		panic(err)
	}

	return &Product{
		pub:  pub,
		opts: opt,
	}
}

// Product message
//
// @param c
// @param message 		key for message
//
// @return err
func (d *Product) Product(c context.Context, message []byte) (err error) {
	m := &sarama.ProducerMessage{
		Topic: d.opts.topic,
		Value: sarama.ByteEncoder(message),
	}

	_, _, err = d.pub.SendMessage(m)
	return
}
