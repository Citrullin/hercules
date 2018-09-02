package tangle

import (
	"github.com/pkg/errors"

	"../convert"
	"../db"
	"../db/coding"
	"../logs"
	"../transaction"
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
	err := coding.PutBytes(tx, db.AsKey(key, db.KEY_HASH), t.Hash)
	_checkSaveError(t, err)
	err = coding.PutInt64(tx, db.AsKey(key, db.KEY_TIMESTAMP), int64(t.Timestamp))
	_checkSaveError(t, err)
	err = coding.PutBytes(tx, db.AsKey(key, db.KEY_BYTES), (*raw)[:1604])
	_checkSaveError(t, err)
	err = coding.PutInt64(tx, db.AsKey(key, db.KEY_VALUE), t.Value)
	_checkSaveError(t, err)
	err = coding.PutBytes(tx, db.AsKey(key, db.KEY_ADDRESS_HASH), t.Address)
	_checkSaveError(t, err)
	err = coding.PutInt(tx,
		append(db.GetByteKey(t.Bundle, db.KEY_BUNDLE), db.AsKey(key, db.KEY_HASH)...),
		t.CurrentIndex)
	_checkSaveError(t, err)
	err = coding.PutString(tx,
		append(db.GetByteKey(t.Tag, db.KEY_TAG), db.AsKey(key, db.KEY_HASH)...),
		"")
	_checkSaveError(t, err)
	err = coding.PutInt64(tx,
		append(db.GetByteKey(t.Address, db.KEY_ADDRESS), db.AsKey(key, db.KEY_HASH)...),
		t.Value)
	_checkSaveError(t, err)
	err = coding.PutBytes(tx,
		db.AsKey(key, db.KEY_RELATION),
		append(trunkKey, branchKey...))
	_checkSaveError(t, err)
	err = coding.PutBool(tx,
		append(db.AsKey(trunkKey, db.KEY_APPROVEE), db.AsKey(key, db.KEY_HASH)...),
		true)
	_checkSaveError(t, err)
	err = coding.PutBool(tx,
		append(db.AsKey(branchKey, db.KEY_APPROVEE), db.AsKey(key, db.KEY_HASH)...),
		false)
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
