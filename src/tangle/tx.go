package tangle

import (
	"transaction"
	"github.com/dgraph-io/badger"
	"db"
	"logs"
	"convert"
	"github.com/pkg/errors"
)

func _checkSaveError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed saving TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}

func saveTX (tx *transaction.FastTX, raw *[]byte, txn *badger.Txn) (e error) {
	defer func() {
		if err := recover(); err != nil {
			e = errors.New("Failed saving TX!")
		}
	}()
	err := db.Put(db.GetByteKey(tx.Hash, db.KEY_HASH), tx.Hash, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(db.GetByteKey(tx.Hash, db.KEY_TIMESTAMP), tx.Timestamp, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(db.GetByteKey(tx.Hash, db.KEY_BYTES), (*raw)[:1604], nil, txn)
	_checkSaveError(tx, err)
	// TODO: delete after confirmation?:
	err = db.Put(db.GetByteKey(tx.Hash, db.KEY_VALUE), tx.Value, nil, txn)
	_checkSaveError(tx, err)
	// TODO: delete after confirmation?:
	err = db.Put(db.GetByteKey(tx.Hash, db.KEY_ADDRESS_HASH), tx.Address, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.Bundle, db.KEY_BUNDLE),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		tx.CurrentIndex, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.Tag, db.KEY_TAG),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		"", nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.Address, db.KEY_ADDRESS),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		tx.Value, nil, txn)
	_checkSaveError(tx, err)
	// TODO: delete after confirmation?:
	err = db.Put(
		db.GetByteKey(tx.Hash, db.KEY_RELATION),
		append(
			db.GetByteKey(tx.TrunkTransaction, db.KEY_HASH),
			db.GetByteKey(tx.BranchTransaction, db.KEY_HASH)...),
		nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.TrunkTransaction, db.KEY_APPROVEE),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		true, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.BranchTransaction, db.KEY_APPROVEE),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		false, nil, txn)
	_checkSaveError(tx, err)

	err = updateTipsOnNewTransaction(tx, txn)
	_checkSaveError(tx, err)
	return nil
}
