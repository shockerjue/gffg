package transport

import "net"

type Request struct {
	*net.TCPListener
	*net.TCPConn
	length int
	packet []byte
	stamp  int64
}

func (r *Request) Packet() []byte {
	return r.packet
}

func (r *Request) Length() int {
	return r.length
}

func (r *Request) Stamp() int64 {
	return r.stamp
}
