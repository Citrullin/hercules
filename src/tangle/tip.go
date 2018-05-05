package tangle

import (
	"transaction"
	"github.com/dgraph-io/badger"
	"db"
	"log"
	"github.com/jmcvetta/randutil"
	"convert"
	"bytes"
	"encoding/gob"
	"sync"
)

type Tip struct {

}

var tips []randutil.Choice
var TipsLocker = &sync.Mutex{}

func tipOnLoad() {
	loadTips()
}

func loadTips() {
	log.Println("Loading tips...")
	_ = db.DB.View(func(txn *badger.Txn) error {
		kvs := db.GetByPrefix([]byte{db.KEY_TIP}, txn)
		for _, kv := range kvs {
			var value int64
			buf := bytes.NewBuffer(kv.Value)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(value)
			if err != nil {
				key := db.AsKey(kv.Key, db.KEY_BYTES)
				txBytes, err := db.GetBytes(key, txn)
				if err == nil {
					trits := convert.BytesToTrits(txBytes)[:8019]
					tx := transaction.TritsToFastTX(&trits, txBytes)
					TipsLocker.Lock()
					tips = append(tips, randutil.Choice{int(value), tx})
					TipsLocker.Unlock()
				}
			}
		}
		return nil
	})
	log.Printf("    ---> %v\n", len(tips))
}

func addTip (tx *transaction.FastTX) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()
	tips = append(tips, randutil.Choice{int(tx.Value), tx})
}

// TODO: remove tips randomly if more than X tips in memory/DB?
func removeTip (tx *transaction.FastTX) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()
	b := tips[:0]
	for _, x := range tips {
		if !bytes.Equal(x.Item.(*transaction.FastTX).Hash, tx.Hash) {
			b = append(b, x)
		}
	}
	tips = b
}

func updateTipsOnNewTransaction (tx *transaction.FastTX, txn *badger.Txn) {
	approvees := db.GetByPrefix(db.GetByteKey(tx.Hash, db.KEY_APPROVEE), txn)
	if len(approvees) < 1 {
		db.Put(db.GetByteKey(tx.Hash, db.KEY_TIP), tx.Value, nil, txn)
		addTip(tx)
	}
	err := db.Remove(db.GetByteKey(tx.TrunkTransaction, db.KEY_TIP), txn)
	if err == nil {
		removeTip(tx)
	}
	err = db.Remove(db.GetByteKey(tx.BranchTransaction, db.KEY_TIP), txn)
	if err == nil {
		removeTip(tx)
	}
}