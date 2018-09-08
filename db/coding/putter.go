package coding

import (
	"bytes"
	"encoding/gob"
)

type Putter interface {
	PutBytes([]byte, []byte) error
}

func PutBool(p Putter, key []byte, value bool) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	return p.PutBytes(key, buf.Bytes())
}

func PutInt(p Putter, key []byte, value int) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	return p.PutBytes(key, buf.Bytes())
}

func PutInt64(p Putter, key []byte, value int64) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	return p.PutBytes(key, buf.Bytes())
}

func PutString(p Putter, key []byte, value string) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
		return err
	}
	return p.PutBytes(key, buf.Bytes())
}
