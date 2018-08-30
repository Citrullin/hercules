package coding_test

import "time"

var testKey = []byte("test")

type putter struct {
	key   []byte
	value []byte
}

func (p *putter) PutBytes(key, value []byte, ttl *time.Duration) error {
	p.key = key
	p.value = value
	return nil
}
