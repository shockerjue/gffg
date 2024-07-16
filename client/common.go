package client

import (
	"sync/atomic"

	"github.com/shockerjue/gffg/registry"

	"github.com/shockerjue/gffg/transport"
)

const (
	RPC_POOL_SIZE = 8
)

var counter int64

func Sid() int64 {
	return atomic.AddInt64(&counter, 1)
}

type client struct {
	S        *transport.Socket
	instance registry.NodeInstance
	Name     string
	Svrname  string
	Stamp    int64
	Group    string
}

type CallCond struct {
	Ch     chan int
	Code   int32
	Packet []byte
}
