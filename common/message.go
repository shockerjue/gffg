package common

// RPC message interface
type Message interface {
	Unmarshal(dAtA []byte) error
	Marshal() (dAtA []byte, err error)
}
