package db

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/dgraph-io/badger"
)

type BadgerTransaction struct {
	txn *badger.Txn
}

func (bt *BadgerTransaction) PutBytes(key, value []byte, ttl *time.Duration) error {
	if ttl == nil {
		return bt.txn.Set(key, value)
	}
	return bt.txn.SetWithTTL(key, value, *ttl)
}

func (bt *BadgerTransaction) GetBytes(key []byte) ([]byte, error) {
	item, err := bt.txn.Get(key)
	if err != nil {
		if err == badger.ErrRetry {
			return bt.GetBytes(key)
		}
		return nil, err
	}

	value, err := item.Value()
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (bt *BadgerTransaction) GetBytesRaw(key []byte) ([]byte, error) {
	data, err := bt.GetBytes(key)
	if err != nil {
		return nil, err
	}
	response := make([]byte, len(data))
	copy(response, data)
	return response, nil
}

func (bt *BadgerTransaction) Has(key []byte) bool {
	_, err := bt.txn.Get(key)
	return err == nil
}

func (bt *BadgerTransaction) Put(key []byte, value interface{}, ttl *time.Duration) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return err
	}
	return bt.PutBytes(key, buf.Bytes(), ttl)
}

func (bt *BadgerTransaction) Get(key []byte, value interface{}) error {
	data, err := bt.GetBytes(key)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(value); err != nil {
		return err
	}
	return nil
}

func (bt *BadgerTransaction) GetString(key []byte) (string, error) {
	var result = ""
	err := bt.Get(key, &result)
	return result, err
}

func (bt *BadgerTransaction) GetInt(key []byte) (int, error) {
	var result = 0
	err := bt.Get(key, &result)
	return result, err
}

func (bt *BadgerTransaction) GetBool(key []byte) (bool, error) {
	var result = false
	err := bt.Get(key, &result)
	return result, err
}

func (bt *BadgerTransaction) GetInt64(key []byte) (int64, error) {
	var result int64 = 0
	err := bt.Get(key, &result)
	return result, err
}

func (bt *BadgerTransaction) Remove(key []byte) error {
	return bt.txn.Delete(key)
}

func (bt *BadgerTransaction) RemoveKeyCategory(keyCategory byte) error {
	return bt.RemovePrefix([]byte{keyCategory})
}

func (bt *BadgerTransaction) RemoveKeysFromCategoryBefore(keyCategory byte, timestamp int64) int {
	var keys [][]byte
	bt.forPrefix([]byte{keyCategory}, true, func(item *badger.Item) {
		value, err := item.Value()
		if err != nil {
			return
		}

		var ts int64
		buf := bytes.NewBuffer(value)
		dec := gob.NewDecoder(buf)
		if err := dec.Decode(&ts); err != nil {
			return
		}
		if ts < timestamp {
			keys = append(keys, AsKey(item.Key(), keyCategory))
		}
	})

	for _, key := range keys {
		bt.Remove(key)
	}

	return len(keys)
}

func (bt *BadgerTransaction) RemovePrefix(prefix []byte) error {
	keys := [][]byte{}
	bt.forPrefix(prefix, false, func(item *badger.Item) {
		itemKey := item.Key()
		key := make([]byte, len(itemKey))
		copy(key, itemKey)
		keys = append(keys, key)
	})

	for _, key := range keys {
		if err := bt.Remove(key); err != nil {
			return err
		}
	}

	return nil
}

func (bt *BadgerTransaction) CountKeyCategory(keyCategory byte) int {
	return bt.CountPrefix([]byte{keyCategory})
}

func (bt *BadgerTransaction) CountPrefix(prefix []byte) int {
	count := 0
	bt.forPrefix(prefix, false, func(item *badger.Item) {
		count++
	})
	return count
}

func (bt *BadgerTransaction) IncrementBy(key []byte, delta int64, deleteOnZero bool) (int64, error) {
	balance, err := bt.GetInt64(key)
	balance += delta
	if balance == 0 && deleteOnZero && err != nil {
		if err := bt.Remove(key); err != nil {
			return balance, err
		}
	}
	err = bt.Put(key, balance, nil)
	return balance, err
}

func (bt *BadgerTransaction) Discard() {
	bt.txn.Discard()
}

func (bt *BadgerTransaction) Commit() error {
	return bt.txn.Commit(func(e error) {})
}

func (bt *BadgerTransaction) forPrefix(prefix []byte, prefetchValues bool, fn func(*badger.Item)) {
	options := badger.DefaultIteratorOptions
	options.PrefetchValues = prefetchValues
	it := bt.txn.NewIterator(options)
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		fn(it.Item())
	}
}
