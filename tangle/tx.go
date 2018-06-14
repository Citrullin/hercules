package tangle

import (
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/transaction"
)

func SaveTX(tx *transaction.FastTX, raw *[]byte, txn *badger.Txn) (e error) {
	defer func() {
		if err := recover(); err != nil {
			e = errors.New("Failed saving TX!")
		}
	}()
	key := db.GetByteKey(tx.Hash, db.KEY_HASH)
	trunkKey := db.GetByteKey(tx.TrunkTransaction, db.KEY_HASH)
	branchKey := db.GetByteKey(tx.BranchTransaction, db.KEY_HASH)

	err := db.Put(db.AsKey(key, db.KEY_HASH), tx.Hash, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(db.AsKey(key, db.KEY_TIMESTAMP), tx.Timestamp, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(db.AsKey(key, db.KEY_BYTES), (*raw)[:1604], nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(db.AsKey(key, db.KEY_VALUE), tx.Value, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(db.AsKey(key, db.KEY_ADDRESS_HASH), tx.Address, nil, txn)
	_checkSaveError(tx, err)
	err = SaveAddressBytes(tx.Address, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.Bundle, db.KEY_BUNDLE),
			db.AsKey(key, db.KEY_HASH)...),
		tx.CurrentIndex, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.Tag, db.KEY_TAG),
			db.AsKey(key, db.KEY_HASH)...),
		"", nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.GetByteKey(tx.Address, db.KEY_ADDRESS),
			db.AsKey(key, db.KEY_HASH)...),
		tx.Value, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		db.AsKey(key, db.KEY_RELATION),
		append(trunkKey, branchKey...),
		nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.AsKey(trunkKey, db.KEY_APPROVEE),
			db.AsKey(key, db.KEY_HASH)...),
		true, nil, txn)
	_checkSaveError(tx, err)
	err = db.Put(
		append(
			db.AsKey(branchKey, db.KEY_APPROVEE),
			db.AsKey(key, db.KEY_HASH)...),
		false, nil, txn)
	_checkSaveError(tx, err)

	err = updateTipsOnNewTransaction(tx, txn)
	_checkSaveError(tx, err)
	return nil
}

func SaveAddressBytes(hash []byte, txn *badger.Txn) error {
	// TODO: (OPT) Use {key byte}+whole address in spends and balances to save even more space?
	key := db.GetByteKey(hash, db.KEY_ADDRESS_BYTES)
	hash, err := db.GetBytesRaw(key, txn)
	if err != nil && len(hash) == 49 {
		return nil
	}
	return db.PutBytes(key, hash, nil, txn)
}

func _checkSaveError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed saving TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}
