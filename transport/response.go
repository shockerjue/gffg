package transport

import (
	"net"
	"sync"
)

type Response struct {
	*net.TCPConn
	rw    sync.Mutex
	stamp int64
}

func (r *Response) Stamp() int64 {
	return r.stamp
}

func (r *Response) Write(packet []byte) (n int, err error) {
	if 0 == len(packet) {
		return
	}

	if nil == r.TCPConn {
		return
	}

	header := &Header{}
	header.Size = int64(len(packet))
	msgHeader, err := header.Encoder()
	if nil != err {
		return
	}
	toWrite := append(msgHeader, packet...)

	r.rw.Lock()
	defer r.rw.Unlock()

	bytesWritten := 0
	toWriteLen := len(toWrite)
	for n < toWriteLen && err == nil {
		bytesWritten, err = r.TCPConn.Write(toWrite[n:])
		if nil != err {
			return
		}

		n += bytesWritten
	}

	return
}
