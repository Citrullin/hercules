package tangle

import (
	"github.com/pkg/errors"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../transaction"
)

func SaveTX(t *transaction.FastTX, raw *[]byte, tx db.Transaction) (e error) {
	defer func() {
		if err := recover(); err != nil {
			e = errors.New("Failed saving TX!")
		}
	}()
	key := ns.HashKey(t.Hash, ns.NamespaceHash)
	trunkKey := ns.HashKey(t.TrunkTransaction, ns.NamespaceHash)
	branchKey := ns.HashKey(t.BranchTransaction, ns.NamespaceHash)

	// TODO: check which of these are still needed. Maybe just bytes can be used...
	err := tx.PutBytes(key, t.Hash)
	_checkSaveError(t, err)

	err = coding.PutInt64(tx, ns.Key(key, ns.NamespaceTimestamp), int64(t.Timestamp))
	_checkSaveError(t, err)

	err = tx.PutBytes(ns.Key(key, ns.NamespaceBytes), (*raw)[:1604])
	_checkSaveError(t, err)

	err = coding.PutInt64(tx, ns.Key(key, ns.NamespaceValue), t.Value)
	_checkSaveError(t, err)

	err = tx.PutBytes(ns.Key(key, ns.NamespaceAddressHash), t.Address)
	_checkSaveError(t, err)

	err = coding.PutInt(tx,
		append(ns.HashKey(t.Bundle, ns.NamespaceBundle), ns.Key(key, ns.NamespaceHash)...),
		t.CurrentIndex)
	_checkSaveError(t, err)

	err = coding.PutString(tx,
		append(ns.HashKey(t.Tag, ns.NamespaceTag), ns.Key(key, ns.NamespaceHash)...),
		"")
	_checkSaveError(t, err)

	err = coding.PutInt64(tx,
		append(ns.HashKey(t.Address, ns.NamespaceAddress), ns.Key(key, ns.NamespaceHash)...),
		t.Value)
	_checkSaveError(t, err)

	err = tx.PutBytes(
		ns.Key(key, ns.NamespaceRelation),
		append(trunkKey, branchKey...))
	_checkSaveError(t, err)

	err = coding.PutBool(tx,
		append(ns.Key(trunkKey, ns.NamespaceApprovee), ns.Key(key, ns.NamespaceHash)...),
		true)
	_checkSaveError(t, err)

	err = coding.PutBool(tx,
		append(ns.Key(branchKey, ns.NamespaceApprovee), ns.Key(key, ns.NamespaceHash)...),
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
