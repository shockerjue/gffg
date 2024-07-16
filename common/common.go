package common

import (
	"context"
	"hash/fnv"
	"net"

	"github.com/google/uuid"
)

// Generate the corresponding signature
// according to the request method name
//
// @param	mothed 	request method name
// @return  request id
func GenRid(mothed string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(mothed))

	return h.Sum64()
}

// Generator uuid
func GenUid() string {
	return uuid.New().String()
}

// Get Ethernet IP
func GetEthIp() (ip string, err error) {
	addrs, err := net.InterfaceAddrs()
	if nil != err {
		return
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ip = ipnet.IP.String()

				return
			}
		}
	}

	return
}

// Set the traceid of the request
//
// @param 	ctx 	Request context
// @param	traceId Request traceid
func SetTraceId(ctx context.Context, traceId string) context.Context {
	return context.WithValue(ctx, "traceId", traceId)
}

// Get traceid for RPC request
func GetTraceId(ctx context.Context) string {
	traceId, ok := ctx.Value("traceId").(string)
	if !ok {
		return ""
	}

	return traceId
}

// Check if the channel is closed
//
// @param	ch 	channel for check
func ClosedChanInt(ch chan int) bool {
	select {
	case _, received := <-ch:
		return !received
	default:
	}

	return false
}
