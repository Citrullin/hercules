package ns

import (
	"github.com/minio/highwayhash"
)

var (
	hashKey = []byte{0xA9, 0x05, 0xDD, 0x48, 0x88, 0x21, 0x5F, 0x3E, 0xD5, 0x1F, 0x5D, 0x9F, 0xCA, 0x51, 0x8E, 0x83,
		0xDD, 0x55, 0x20, 0xE8, 0x76, 0x38, 0xC0, 0x08, 0xB9, 0x78, 0xF8, 0xAF, 0x20, 0x55, 0x93, 0xAF}
)

// Returns a 16-bytes key based on other key
func Key(key []byte, namespace byte) []byte {
	b := make([]byte, 16)
	copy(b, key)
	b[0] = namespace
	return b
}

// Returns a 16-bytes key based on bytes
func HashKey(bytes []byte, namespace byte) []byte {
	b := highwayhash.Sum128(bytes, hashKey)
	b[0] = namespace
	return b[:]
}

// Returns a 16-bytes key based on bytes
func AddressKey(address []byte, namespace byte) []byte {
	b := make([]byte, 50)
	b[0] = namespace
	copy(b[1:], address)
	return b
}
