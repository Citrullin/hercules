package tangle

import (
	"transaction"
	"github.com/dgraph-io/badger"
	"db"
)

func saveTX (tx *transaction.FastTX, raw *[]byte, txn *badger.Txn) {
	db.Put(db.GetByteKey(tx.Hash, db.KEY_HASH), tx.Hash, nil, txn)
	db.Put(db.GetByteKey(tx.Hash, db.KEY_TIMESTAMP), tx.Timestamp, nil, txn)
	db.Put(db.GetByteKey(tx.Hash, db.KEY_BYTES), (*raw)[:1604], nil, txn)
	db.Put(db.GetByteKey(tx.Hash, db.KEY_VALUE), tx.Value, nil, txn)
	db.Put(db.GetByteKey(tx.Hash, db.KEY_ADDRESS_HASH), tx.Address, nil, txn)
	db.Put(
		append(
			db.GetByteKey(tx.Bundle, db.KEY_BUNDLE),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		tx.CurrentIndex, nil, txn)
	db.Put(
		append(
			db.GetByteKey(tx.Tag, db.KEY_TAG),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		"", nil, txn)
	db.Put(
		append(
			db.GetByteKey(tx.Address, db.KEY_ADDRESS),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		tx.Value, nil, txn)
	db.Put(
		db.GetByteKey(tx.Hash, db.KEY_RELATION),
		append(
			db.GetByteKey(tx.TrunkTransaction, db.KEY_HASH),
			db.GetByteKey(tx.BranchTransaction, db.KEY_HASH)...),
		nil, txn)
	db.Put(
		append(
			db.GetByteKey(tx.TrunkTransaction, db.KEY_APPROVEE),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		"", nil, txn)
	db.Put(
		append(
			db.GetByteKey(tx.BranchTransaction, db.KEY_APPROVEE),
			db.GetByteKey(tx.Hash, db.KEY_HASH)...),
		"", nil, txn)

	updateTipsOnNewTransaction(tx, txn)
}
