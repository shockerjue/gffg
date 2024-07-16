package transport

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

type Listener struct {
	socket          *net.TCPListener
	shutdownChannel chan struct{}
	shutdownGroup   *sync.WaitGroup

	opts *options
}

func NewListener(opts ...TransOption) (*Listener, error) {
	cfg := initOpts(opts...)
	if cfg.maxMessageSize == 0 {
		cfg.maxMessageSize = int32(DefaultMaxMessageSize)
	}
	if cfg.headerByteSize == 0 {
		cfg.headerByteSize = DefaultHeaderSize
	}

	btl := &Listener{
		shutdownChannel: make(chan struct{}),
		shutdownGroup:   &sync.WaitGroup{},
		opts:            cfg,
	}

	if err := btl.openSocket(); err != nil {
		return nil, err
	}

	return btl, nil
}

func (btl *Listener) blockListen() error {
	for {
		conn, err := btl.socket.AcceptTCP()
		if err != nil {
			select {
			case <-btl.shutdownChannel:
				return err

			default:
			}

			zzlog.Errorw("Listener.blockListen error", zap.Error(err))
			continue
		}

		conn.SetReadDeadline(time.Now().Add(1800 * time.Second))
		// conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		skt, _ := SocketByConn(conn)
		if nil != btl.opts.event.Connect {
			btl.opts.event.Connect(context.TODO(), &Request{TCPConn: conn})
		}
		go skt.onRecv(
			int(btl.opts.headerByteSize),
			int(btl.opts.maxMessageSize),
			btl.opts.event.OnRecv,
			btl.opts.event.Closed)
	}
}

func (btl *Listener) openSocket() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", btl.opts.address)
	if err != nil {
		return err
	}
	receiveSocket, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}

	if nil != btl.opts.event.Listen {
		btl.opts.event.Listen(context.TODO(), &Request{TCPListener: receiveSocket})
	}

	btl.socket = receiveSocket
	return err
}

func (btl *Listener) Start() error {
	return btl.blockListen()
}

func (btl *Listener) Close() {
	close(btl.shutdownChannel)
	btl.shutdownGroup.Wait()
}

func (btl *Listener) StartAsync() error {
	var err error
	go func() {
		err = btl.blockListen()
	}()
	return err
}
