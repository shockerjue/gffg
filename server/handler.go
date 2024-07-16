package server

import (
	"context"
)

type RpcItem struct {
	Call func(context.Context, []byte) ([]byte, error)
	Name string
}

type rpcHandler struct {
	calls map[uint64]*RpcItem
}

func RpcHandler() *rpcHandler {
	return &rpcHandler{
		calls: make(map[uint64]*RpcItem),
	}
}

func (c *rpcHandler) Add(rid uint64, rpc *RpcItem) {
	c.calls[rid] = rpc
}
