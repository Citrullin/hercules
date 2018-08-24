package db

import (
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

func (bt *BadgerTransaction) Discard() {
	bt.txn.Discard()
}

func (bt *BadgerTransaction) Commit() error {
	return bt.txn.Commit(func(e error) {})
}
