package coding_test

import (
	"bytes"
	"time"
)

var testKey = []byte("test")

type storage struct {
	key   []byte
	value []byte
}

func (s *storage) PutBytes(key, value []byte, ttl *time.Duration) error {
	s.key = key
	s.value = value
	return nil
}

func (s *storage) GetBytes(key []byte) ([]byte, error) {
	if bytes.Equal(s.key, key) {
		return s.value, nil
	}
	return nil, nil
}
