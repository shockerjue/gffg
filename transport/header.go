package transport

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// Message definition, which includes message validation fields
// Used to verify whether the message is from this framework.
// The length of the message header is 8 bytes.
// 4 bytes are the signature.
// 4 bytes are the message body length.
// The packet format is: Sign | Size | packet
type Header struct {
	Sign [4]byte
	Size int64
}

// Encode the packet header and convert it into binary format
//
// @return 	read
// @return  err
func (h *Header) Encoder() (read []byte, err error) {
	buf := new(bytes.Buffer)

	type msgHeader struct {
		S1 [4]byte
		S2 [4]byte
	}
	var msg msgHeader

	binary.PutVarint(msg.S2[:], h.Size)

	rsa := [4]byte{msg.S2[1], msg.S2[3], msg.S2[2], msg.S2[0]}

	copy(msg.S1[:], rsa[:])
	err = binary.Write(buf, binary.LittleEndian, &msg)
	if err != nil {
		return
	}

	read = buf.Bytes()
	return
}

// Decode the packet header and decode the data into struct
//
// @param	buf
// @return	err
func (h *Header) Decoder(buf []byte) (err error) {
	type msgHeader struct {
		S1 [4]byte
		S2 [4]byte
	}

	var msg msgHeader
	reader := bytes.NewReader(buf)
	err = binary.Read(reader, binary.LittleEndian, &msg)
	if err != nil {
		return
	}

	h.Sign = msg.S1

	var n int
	h.Size, n = binary.Varint(msg.S2[:])
	if 0 == n {
		err = errors.New("Decode header2 fail")

		return
	}

	return
}

// Verify that the message body is correct
//
// @param	sign
// @return  err
func (h *Header) Check() (err error) {
	sign := [4]byte{h.Sign[3], h.Sign[0], h.Sign[2], h.Sign[1]}
	value, _ := binary.Varint(sign[:])
	if value == h.Size {
		return
	}

	err = errors.New("Sign check fail")
	return
}
