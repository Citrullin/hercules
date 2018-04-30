package tangle

import (
	"db"
	"github.com/dgraph-io/badger"
	"convert"
	"time"
	"bytes"
)

func (tx *FastTX) confirm (txn *badger.Txn) {
	db.Remove(db.GetByteKey(tx.Hash, db.KEY_UNKNOWN), txn)
	db.Put(db.GetByteKey(tx.Hash, db.KEY_CONFIRMED), tx.Timestamp, nil, txn)
	if tx.Value > 0 {
		_, err := db.IncrBy(db.GetByteKey(tx.Hash, db.KEY_ACCOUNT), tx.Value, true, txn)
		if err != nil {
			panic("Could not update account balance!")
		}
	}
	confirmChild(tx.TrunkTransaction, txn)
	confirmChild(tx.BranchTransaction, txn)
}

func confirmChild (hash []byte, txn *badger.Txn) {
	if db.Has(db.GetByteKey(hash, db.KEY_CONFIRMED), txn) { return }
	txBytes, err := db.GetBytes(db.GetByteKey(hash, db.KEY_TRANSACTION), txn)
	if err != nil && len(txBytes) > 0 {
		trits := convert.BytesToTrits(txBytes)[:8019]
		tx := TritsToFastTX(&trits)
		if tx != nil {
			tx.confirm(txn)
		}
	} else {
		db.Put(db.GetByteKey(hash, db.KEY_UNKNOWN), int(time.Now().Unix()), nil, nil)
	}

}

func isMilestone(tx *FastTX) bool {
	// TODO: check if really milestone
	return bytes.Equal(tx.Address, coo)
}
