package kafka

import (
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/IBM/sarama"
)

type Consumer struct {
	consumer sarama.Consumer

	opts *options
}

// Create kafka consumer
//
// @param opts Options
func NewConsumer(opts ...Options) *Consumer {
	opt := &options{}
	for _, o := range opts {
		o(opt)
	}

	brokers := strings.Split(opt.brokers, ",")

	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Version = sarama.V2_6_0_0
	sub, err := sarama.NewConsumer(brokers, config)
	if err != nil {
		panic(err)
	}
	return &Consumer{
		consumer: sub,
		opts:     opt,
	}
}

func (j *Consumer) Release() {
	j.consumer.Close()
}

// loop consume message
//
// @param errf 	call for error
// @param notif call for notify
// @param msg   call for message
func (j *Consumer) Consume(icb IConsumer) {
	partitions, err := j.consumer.Partitions(j.opts.topic)
	if err != nil {
		icb.Error(err)

		return
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	var wg sync.WaitGroup
	parts := make([]sarama.PartitionConsumer, 0)
	for _, partition := range partitions {
		partitions, err := j.consumer.ConsumePartition(j.opts.topic,
			partition, sarama.OffsetNewest)
		if err != nil {
			icb.Error(err)

			return
		}
		parts = append(parts, partitions)

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case msg := <-partitions.Messages():
					icb.Message(msg.Partition, msg.Offset, msg.Value)
				case err := <-partitions.Errors():
					icb.Error(err)
					return
				case <-signals:
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, it := range parts {
		it.Close()
	}
}
