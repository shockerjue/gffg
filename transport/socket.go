package transport

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/shockerjue/gffg/zzlog"
	"go.uber.org/zap"
)

const (
	// Default size for message header
	DefaultHeaderSize = 8
	// Max message size
	DefaultMaxMessageSize = int(1 << 20)
)

type Socket struct {
	conn     *net.TCPConn
	response *Response
	request  *Request
}

func SocketByAddr(addr string) (s *Socket, err error) {
	s = &Socket{
		response: &Response{},
		request:  &Request{},
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return
	}
	t, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		return
	}

	t.SetKeepAlive(true)

	s.conn = t
	s.response.TCPConn = t
	s.request.TCPConn = t
	return
}

func SocketByConn(t *net.TCPConn) (s *Socket, err error) {
	s = &Socket{
		response: &Response{},
		request:  &Request{},
	}

	t.SetKeepAlive(true)

	s.conn = t
	s.response.TCPConn = t
	s.request.TCPConn = t
	return
}

func (s *Socket) Close() {
	if nil != s.conn {
		s.conn.Close()
	}
}

func (s *Socket) Conn() *net.TCPConn {
	return s.conn
}

func (s *Socket) Response() *Response {
	return s.response
}

func (s *Socket) Request() *Request {
	return s.request
}

func (s *Socket) Handle(rcb func(context.Context, *Request, *Response) error,
	ccb func(context.Context, *Request) error) (err error) {
	s.onRecv(DefaultHeaderSize, DefaultMaxMessageSize, rcb, ccb)

	return
}

func (s *Socket) onRecv(headerByteSize int, maxMessageSize int,
	rcb func(context.Context, *Request, *Response) error,
	ccb func(context.Context, *Request) error) {
	headerBuffer := make([]byte, headerByteSize)
	dataBuffer := make([]byte, maxMessageSize)
	defer func() {
		if err := recover(); nil != err {
			zzlog.Errorw("onRecv except", zap.Error(err.(error)))
		}

		if nil != s.conn {
			zzlog.Errorw("Client closed connection", zap.String("Address",
				s.conn.RemoteAddr().String()))

			s.conn.Close()
		}

		if nil != ccb {
			ccb(context.TODO(), &Request{TCPConn: s.conn})
		}

		return
	}()

	for {
		// Read the message header
		var totalHeaderBytesRead = 0
		for totalHeaderBytesRead < headerByteSize {
			bytesRead, err := s.readFromConnection(s.conn, headerBuffer[totalHeaderBytesRead:])
			if err != nil {
				if err != io.EOF {
					zzlog.Errorw("Error when trying to read",
						zap.String("address", s.conn.RemoteAddr().String()),
						zap.Int("headerByteSize", headerByteSize),
						zap.Int("totalHeaderBytesRead", totalHeaderBytesRead),
						zap.Error(err))
				} else {
					zzlog.Errorw("Client closed connection during header read. Underlying error",
						zap.String("address", s.conn.RemoteAddr().String()), zap.Error(err))
				}

				return
			}

			totalHeaderBytesRead += bytesRead
		}

		// Decode the message header and verify
		var header Header
		err := header.Decoder(headerBuffer)
		if nil != err {
			zzlog.Errorw("Decoder header error ==========>", zap.Any("headerBuffer", len(headerBuffer)),
				zap.Any("headerByteSize", headerByteSize), zap.String("address",
					s.conn.RemoteAddr().String()), zap.Error(err))

			return
		}

		err = header.Check()
		if nil != err {
			zzlog.Errorw("Check header error ==========> ",
				zap.String("address", s.conn.RemoteAddr().String()), zap.Error(err))

			return
		}

		iMsgLength := int(header.Size)

		// Read the entire message body, the message length is iMsgLength
		var totalDataBytesRead = 0
		for totalDataBytesRead < iMsgLength {
			bytesRead, err := s.readFromConnection(s.conn, dataBuffer[totalDataBytesRead:iMsgLength])
			if err != nil {
				if err != io.EOF {
					zzlog.Errorw("Failure to read from connection. ",
						zap.String("address", s.conn.RemoteAddr().String()),
						zap.Int("msgLength", iMsgLength),
						zap.Int("totalDataBytesRead", totalDataBytesRead),
						zap.Error(err))
				} else {
					zzlog.Errorw("Client closed connection during data read. Underlying error",
						zap.String("address", s.conn.RemoteAddr().String()), zap.Error(err))
				}

				return
			}

			totalDataBytesRead += bytesRead
		}
		if 0 == totalDataBytesRead {
			continue
		}

		// Prevent sticking
		// If there is no error in reading the message, the callback function is called
		packet := make([]byte, iMsgLength)
		copy(packet, dataBuffer[:iMsgLength])

		stamp := time.Now().UnixMilli()
		request := &Request{
			TCPConn: s.conn,
			length:  iMsgLength,
			packet:  packet,
			stamp:   stamp,
		}
		err = rcb(context.TODO(), request, &Response{
			TCPConn: s.conn,
			stamp:   stamp,
		})
		if err != nil {
			zzlog.Errorw("Socket recv.Callback error", zap.Error(err))
		}
	}
}

// Handles reading from a given connection.
func (s *Socket) readFromConnection(reader *net.TCPConn, buffer []byte) (int, error) {
	// This fills the buffer
	bytesLen, err := reader.Read(buffer)
	if err != nil {
		//"Underlying network failure?"
		// Not sure what this error would be, but it could exist and i've seen it handled
		// as a general case in other networking code. Following in the footsteps of (greatness|madness)
		return bytesLen, err
	}

	// Read some bytes, return the length
	return bytesLen, nil
}
