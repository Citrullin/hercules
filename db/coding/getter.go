package coding

import (
	"bytes"
	"encoding/gob"
)

type Getter interface {
	GetBytes([]byte) ([]byte, error)
}

func GetBytes(g Getter, key []byte) ([]byte, error) {
	value, err := g.GetBytes(key)
	if err != nil {
		return nil, err
	}

	var result = []byte{}
	if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func GetBool(g Getter, key []byte) (bool, error) {
	value, err := g.GetBytes(key)
	if err != nil {
		return false, err
	}

	var result = false
	if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&result); err != nil {
		return false, err
	}
	return result, nil
}

func GetInt(g Getter, key []byte) (int, error) {
	value, err := g.GetBytes(key)
	if err != nil {
		return 0, err
	}

	var result = 0
	if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&result); err != nil {
		return 0, err
	}
	return result, nil
}

func GetInt64(g Getter, key []byte) (int64, error) {
	value, err := g.GetBytes(key)
	if err != nil {
		return 0, err
	}

	var result = int64(0)
	if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&result); err != nil {
		return 0, err
	}
	return result, nil
}

func GetString(g Getter, key []byte) (string, error) {
	value, err := g.GetBytes(key)
	if err != nil {
		return "", err
	}

	var result = ""
	if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&result); err != nil {
		return "", err
	}
	return result, nil
}
