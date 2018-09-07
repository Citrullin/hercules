package ns_test

import "bytes"

var (
	testKey   = []byte("key")
	testValue = []byte("value")
)

type storage struct {
	key   []byte
	value []byte
}

func (s *storage) CountPrefix(prefix []byte) int {
	if bytes.HasPrefix(s.key, prefix) {
		return 1
	}
	return 0
}

func (s *storage) RemovePrefix(prefix []byte) error {
	if bytes.HasPrefix(s.key, prefix) {
		s.key = nil
		s.value = nil
	}
	return nil
}
