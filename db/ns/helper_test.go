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
