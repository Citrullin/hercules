package coding

import (
	"encoding/binary"
)

type Putter interface {
	PutBytes([]byte, []byte) error
}

func PutBool(p Putter, key []byte, value bool) error {
	if value {
		return p.PutBytes(key, []byte{0x01})
	}
	return p.PutBytes(key, []byte{0x00})
}

func PutInt(p Putter, key []byte, value int) error {
	var buffer [binary.MaxVarintLen64]byte
	binary.PutVarint(buffer[:], int64(value))
	return p.PutBytes(key, buffer[:])
}

func PutInt64(p Putter, key []byte, value int64) error {
	var buffer [binary.MaxVarintLen64]byte
	binary.PutVarint(buffer[:], value)
	return p.PutBytes(key, buffer[:])
}

func PutString(p Putter, key []byte, value string) error {
	return p.PutBytes(key, []byte(value))
}
