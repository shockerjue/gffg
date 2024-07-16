package transport

import "context"

// transport event interface
type TransEvent struct {
	Listen  func(context.Context, *Request) error
	Connect func(context.Context, *Request) error
	Closed  func(context.Context, *Request) error
	OnRecv  func(context.Context, *Request, *Response) error
}
