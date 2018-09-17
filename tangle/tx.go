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

func SaveTX(tx *transaction.FastTX, raw *[]byte, dbTx db.Transaction) (e error) {
	defer func() {
		if err := recover(); err != nil {
			e = errors.New("Failed saving TX!")
		}
	}()
	key := ns.HashKey(tx.Hash, ns.NamespaceHash)
	trunkKey := ns.HashKey(tx.TrunkTransaction, ns.NamespaceHash)
	branchKey := ns.HashKey(tx.BranchTransaction, ns.NamespaceHash)

	// TODO: check which of these are still needed. Maybe just bytes can be used...
	err := dbTx.PutBytes(key, tx.Hash)
	_checkSaveError(tx, err)

	err = coding.PutInt64(dbTx, ns.Key(key, ns.NamespaceTimestamp), int64(tx.Timestamp))
	_checkSaveError(tx, err)

	err = dbTx.PutBytes(ns.Key(key, ns.NamespaceBytes), (*raw)[:1604])
	_checkSaveError(tx, err)

	err = coding.PutInt64(dbTx, ns.Key(key, ns.NamespaceValue), tx.Value)
	_checkSaveError(tx, err)

	err = dbTx.PutBytes(ns.Key(key, ns.NamespaceAddressHash), tx.Address)
	_checkSaveError(tx, err)

	err = coding.PutInt(dbTx,
		append(ns.HashKey(tx.Bundle, ns.NamespaceBundle), ns.Key(key, ns.NamespaceHash)...),
		tx.CurrentIndex)
	_checkSaveError(tx, err)

	err = coding.PutString(dbTx,
		append(ns.HashKey(tx.Tag, ns.NamespaceTag), ns.Key(key, ns.NamespaceHash)...),
		"")
	_checkSaveError(tx, err)

	err = coding.PutInt64(dbTx,
		append(ns.HashKey(tx.Address, ns.NamespaceAddress), ns.Key(key, ns.NamespaceHash)...),
		tx.Value)
	_checkSaveError(tx, err)

	err = dbTx.PutBytes(
		ns.Key(key, ns.NamespaceRelation),
		append(trunkKey, branchKey...))
	_checkSaveError(tx, err)

	err = coding.PutBool(dbTx,
		append(ns.Key(trunkKey, ns.NamespaceApprovee), ns.Key(key, ns.NamespaceHash)...),
		true)
	_checkSaveError(tx, err)

	err = coding.PutBool(dbTx,
		append(ns.Key(branchKey, ns.NamespaceApprovee), ns.Key(key, ns.NamespaceHash)...),
		false)
	_checkSaveError(tx, err)

	err = updateTipsOnNewTransaction(tx, dbTx)
	_checkSaveError(tx, err)

	return nil
}

func _checkSaveError(tx *transaction.FastTX, err error) {
	if err != nil {
		logs.Log.Errorf("Failed saving TX %v", convert.BytesToTrytes(tx.Hash)[:81], err)
		panic(err)
	}
}
