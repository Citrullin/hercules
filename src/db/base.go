package db

import (
	"github.com/dgraph-io/badger"
	"convert"
	"crypto/md5"
	"encoding/gob"
	"bytes"
	"time"
)

const (
	KEY_HASH = byte(1) // hash -> tx.hash trytes
	KEY_TIMESTAMP = byte(2) // hash -> time
	KEY_TRANSACTION = byte(3) // hash -> raw tx trytes
	KEY_RELATION = byte(4) // hash -> hash+hash
	KEY_CONFIRMED = byte(5) // hash -> time
	KEY_MILESTONE = byte(6) // hash -> time
	KEY_REQUESTS = byte(20) // hash -> time
	KEY_REQUESTS_HASH = byte(21) // hash -> time
	KEY_UNKNOWN = byte(25) // hash -> time
	KEY_ACCOUNT = byte(100) // hash -> int64
	KEY_SNAPSHOT = byte(120) // hash -> hash+int64+hash+int64+...
	KEY_TEST = byte(187) // hash -> bool
)

// Returns a 16-bytes key based on a string hash
func GetHashKey (hash string, key byte) []byte {
	resp := md5.Sum(convert.TrytesToBytes(hash))
	resp[0] = key
	return resp[:]
}

// Returns a 16-bytes key based on bytes
func GetByteKey (bytes []byte, key byte) []byte {
	b := md5.Sum(bytes)
	b[0] = key
	return b[:]
}

func Has (key []byte, txn *badger.Txn) bool {
	_, err := txn.Get(key)
	return !(err != nil)
}

func Put (key []byte, value interface{}, txn *badger.Txn) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(value)
	if err != nil {
		return err
	}
	return txn.Set(key, buf.Bytes())
}

func Get (key []byte, data interface{}, txn *badger.Txn) error {
	item, err := txn.Get(key)
	if err != nil {
		return err
	}
	val, err := item.Value()
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(val)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(data)
	if err != nil {
		return err
	}
	return nil
}

func GetBytes(key []byte, txn *badger.Txn) ([]byte, error) {
	var resp []byte = nil
	err := Get(key, &resp, txn)
	return resp, err
}

func GetInt(key []byte, txn *badger.Txn) (int, error) {
	var resp int = 0
	err := Get(key, &resp, txn)
	return resp, err
}

func GetInt64(key []byte, txn *badger.Txn) (int64, error) {
	var resp int64 = 0
	err := Get(key, &resp, txn)
	return resp, err
}

func Remove (key []byte, txn *badger.Txn) error {
	return txn.Delete(key)
}

func Count(key byte) int {
	count := 0
	_ = DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{key}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next(){
			count++
		}
		return nil
	})
	return count
}

/*
Returns latest key iterating over all items of certain type.
The value is expected to be a unix timestamp
 */
func GetLatestKey(key byte, txn *badger.Txn) ([]byte, int, error) {
	var latest []byte
	var current = 0
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	prefix := []byte{key}
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next(){
		item := it.Item()
		v, err := item.Value()
		if err != nil {
			return nil, 0, err
		}
		var data int
		buf := bytes.NewBuffer(v)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(data)
		if err != nil {
			return nil, 0, err
		}
		if data > current {
			current = data
			latest = item.Key()
		}
	}
	return latest, current, nil
}

/*
Returns latest random key iterating over all items of certain type.
The value is expected to be a unix timestamp. //When picked, the value is updated
 */
func PickRandomKey(key byte, txn *badger.Txn) []byte {
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)
	defer it.Close()

	pos := 0
	// TODO: when some buffers (like requests) fill up to a certain point, remove half of them, maybe?
	r := random(0, 10000)
	var result []byte

	prefix := []byte{key}
	for it.Seek(prefix); it.ValidForPrefix(prefix) && pos <= r; it.Next(){
		r++
		result = it.Item().Key()
	}
	Put(result, time.Now().Unix(), txn)
	return result
}

func IncrBy(key []byte, value int64, deleteOnZero bool, txn *badger.Txn) (int64, error) {
	balance, err := GetInt64(key, txn)
	balance += value
	if balance == 0 && deleteOnZero && err != nil {
		Remove(key, txn)
	}
	err = Put(key, balance, txn)
	return balance, err
}
