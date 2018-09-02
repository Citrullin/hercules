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
	err := error(nil)
	if ttl == nil {
		err = bt.txn.Set(key, value)
	} else {
		err = bt.txn.SetWithTTL(key, value, *ttl)
	}
	if err == badger.ErrTxnTooBig {
		return ErrTransactionTooBig
	}
	return err
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

func (bt *BadgerTransaction) HasKey(key []byte) bool {
	_, err := bt.txn.Get(key)
	return err == nil
}

func (bt *BadgerTransaction) HasKeysFromCategoryBefore(keyCategory byte, timestamp int64) bool {
	result := false
	bt.ForPrefix([]byte{keyCategory}, true, func(_, value []byte) (bool, error) {
		var ts = int64(0)
		if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&ts); err != nil {
			return false, err
		}
		if ts > 0 && ts <= timestamp {
			result = true
			return false, nil
		}
		return true, nil
	})
	return result
}

func (bt *BadgerTransaction) Put(key []byte, value interface{}, ttl *time.Duration) error {

	switch value.(type) {
	case []byte:
		return bt.PutBytes(key, value.([]byte), ttl)
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(value); err != nil {
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
	err := bt.txn.Delete(key)
	if err == badger.ErrTxnTooBig {
		return ErrTransactionTooBig
	}
	return err
}

func (bt *BadgerTransaction) RemoveKeyCategory(keyCategory byte) error {
	return bt.RemovePrefix([]byte{keyCategory})
}

func (bt *BadgerTransaction) RemoveKeysFromCategoryBefore(keyCategory byte, timestamp int64) int {
	var keys [][]byte
	bt.ForPrefix([]byte{keyCategory}, true, func(key, value []byte) (bool, error) {
		var ts int64
		if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&ts); err != nil {
			return false, err
		}
		if ts < timestamp {
			keys = append(keys, AsKey(key, keyCategory))
		}

		return true, nil
	})

	for _, key := range keys {
		bt.Remove(key)
	}

	return len(keys)
}

func (bt *BadgerTransaction) RemovePrefix(prefix []byte) error {
	keys := [][]byte{}
	bt.ForPrefix(prefix, false, func(itemKey, _ []byte) (bool, error) {
		key := make([]byte, len(itemKey))
		copy(key, itemKey)
		keys = append(keys, key)
		return true, nil
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
	bt.ForPrefix(prefix, false, func(_, _ []byte) (bool, error) {
		count++
		return true, nil
	})
	return count
}

func (bt *BadgerTransaction) SumInt64FromCategory(keyCategory byte) int64 {
	sum := int64(0)
	bt.ForPrefix([]byte{keyCategory}, true, func(_, value []byte) (bool, error) {
		var v int64 = 0
		if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&v); err != nil {
			return false, err
		}
		sum += v

		return true, nil
	})
	return sum
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

func (bt *BadgerTransaction) ForPrefix(prefix []byte, fetchValues bool, fn func([]byte, []byte) (bool, error)) error {
	options := badger.DefaultIteratorOptions
	options.PrefetchValues = fetchValues
	it := bt.txn.NewIterator(options)
	defer it.Close()

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		ok, err := false, error(nil)
		if fetchValues {
			value, err := it.Item().Value()
			if err != nil {
				return err
			}
			ok, err = fn(it.Item().Key(), value)
		} else {
			ok, err = fn(it.Item().Key(), nil)
		}
		if err != nil {
			return err
		}
		if !ok {
			break
		}
	}

	return nil
}
