package tangle

import (
	"../convert"
	"../db"
	"../logs"
	"../transaction"
	"github.com/pkg/errors"
)

func SaveTX(t *transaction.FastTX, raw *[]byte, tx db.Transaction) (e error) {
	defer func() {
		if err := recover(); err != nil {
			e = errors.New("Failed saving TX!")
		}
	}()
	key := db.GetByteKey(t.Hash, db.KEY_HASH)
	trunkKey := db.GetByteKey(t.TrunkTransaction, db.KEY_HASH)
	branchKey := db.GetByteKey(t.BranchTransaction, db.KEY_HASH)

	// TODO: check which of these are still needed. Maybe just bytes can be used...
	err := tx.Put(db.AsKey(key, db.KEY_HASH), t.Hash, nil)
	_checkSaveError(t, err)
	err = tx.Put(db.AsKey(key, db.KEY_TIMESTAMP), t.Timestamp, nil)
	_checkSaveError(t, err)
	err = tx.Put(db.AsKey(key, db.KEY_BYTES), (*raw)[:1604], nil)
	_checkSaveError(t, err)
	err = tx.Put(db.AsKey(key, db.KEY_VALUE), t.Value, nil)
	_checkSaveError(t, err)
	err = tx.Put(db.AsKey(key, db.KEY_ADDRESS_HASH), t.Address, nil)
	_checkSaveError(t, err)
	err = tx.Put(
		append(
			db.GetByteKey(t.Bundle, db.KEY_BUNDLE),
			db.AsKey(key, db.KEY_HASH)...),
		t.CurrentIndex, nil)
	_checkSaveError(t, err)
	err = tx.Put(
		append(
			db.GetByteKey(t.Tag, db.KEY_TAG),
			db.AsKey(key, db.KEY_HASH)...),
		"", nil)
	_checkSaveError(t, err)
	err = tx.Put(
		append(
			db.GetByteKey(t.Address, db.KEY_ADDRESS),
			db.AsKey(key, db.KEY_HASH)...),
		t.Value, nil)
	_checkSaveError(t, err)
	err = tx.Put(
		db.AsKey(key, db.KEY_RELATION),
		append(trunkKey, branchKey...),
		nil)
	_checkSaveError(t, err)
	err = tx.Put(
		append(
			db.AsKey(trunkKey, db.KEY_APPROVEE),
			db.AsKey(key, db.KEY_HASH)...),
		true, nil)
	_checkSaveError(t, err)
	err = tx.Put(
		append(
			db.AsKey(branchKey, db.KEY_APPROVEE),
			db.AsKey(key, db.KEY_HASH)...),
		false, nil)
	_checkSaveError(t, err)

	err = updateTipsOnNewTransaction(t, tx)
	_checkSaveError(t, err)
	return nil
}

func _checkSaveError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed saving TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}
