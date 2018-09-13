package coding

import (
	"encoding/binary"
)

type Getter interface {
	GetBytes([]byte) ([]byte, error)
}

func GetBool(g Getter, key []byte) (bool, error) {
	value, err := g.GetBytes(key)
	if err != nil {
		return false, err
	}
	if len(value) > 0 && value[0] != 0x00 {
		return true, nil
	}
	return false, nil
}

func GetInt(g Getter, key []byte) (int, error) {
	buffer, err := g.GetBytes(key)
	if err != nil {
		return 0, err
	}
	value, _ := binary.Varint(buffer)
	return int(value), nil
}

func GetInt64(g Getter, key []byte) (int64, error) {
	buffer, err := g.GetBytes(key)
	if err != nil {
		return 0, err
	}
	value, _ := binary.Varint(buffer)
	return value, nil
}

func GetString(g Getter, key []byte) (string, error) {
	value, err := g.GetBytes(key)
	if err != nil {
		return "", err
	}
	return string(value), nil
}
