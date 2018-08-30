package coding

import (
	"bytes"
	"encoding/gob"
	"time"
)

type Putter interface {
	PutBytes([]byte, []byte, *time.Duration) error
}

func PutBool(p Putter, key []byte, value bool) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	return p.PutBytes(key, buf.Bytes(), nil)
}

func PutInt64(p Putter, key []byte, value int64) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	return p.PutBytes(key, buf.Bytes(), nil)
}

func PutString(p Putter, key []byte, value string) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	return p.PutBytes(key, buf.Bytes(), nil)
}
