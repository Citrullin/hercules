package db

import (
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
