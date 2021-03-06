package coding_test

import (
	"bytes"
)

var (
	testKey = []byte("test")
)

type storage struct {
	key   []byte
	value []byte
}

func (s *storage) PutBytes(key, value []byte) error {
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

func (s *storage) ForPrefix(prefix []byte, fetchValues bool, fn func([]byte, []byte) (bool, error)) error {
	if bytes.HasPrefix(s.key, prefix) {
		if fetchValues {
			_, err := fn(s.key, s.value)
			return err
		}
		_, err := fn(s.key, nil)
		return err
	}
	return nil
}

func (s *storage) Remove(key []byte) error {
	s.key = nil
	s.value = nil
	return nil
}
