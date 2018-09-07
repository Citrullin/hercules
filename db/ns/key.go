package ns

import "crypto/md5"

// Returns a 16-bytes key based on other key
func Key(key []byte, namespace byte) []byte {
	b := make([]byte, 16)
	copy(b, key)
	b[0] = namespace
	return b
}

// Returns a 16-bytes key based on bytes
func HashKey(bytes []byte, namespace byte) []byte {
	b := md5.Sum(bytes)
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
