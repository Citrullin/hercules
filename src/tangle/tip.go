package tangle

import (
	"transaction"
	"github.com/dgraph-io/badger"
	"db"
	"convert"
	"bytes"
	"encoding/gob"
	"sync"
	"math"
	"utils"
	"logs"
)

type Tip struct {
	Value int64
	TX *transaction.FastTX
}

var tips []Tip
var TipsLocker = &sync.Mutex{}

func tipOnLoad() {
	loadTips()
}

func loadTips() {
	logs.Log.Info("Loading tips...")
	_ = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_TIP}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			v, _ := item.Value()

			var value int64
			buf := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&value)
			if err == nil {
				key := db.AsKey(key, db.KEY_BYTES)
				txBytes, err := db.GetBytes(key, txn)
				if err == nil {
					trits := convert.BytesToTrits(txBytes)[:8019]
					tx := transaction.TritsToFastTX(&trits, txBytes)
					TipsLocker.Lock()
					tips = append(tips, Tip{value, tx})
					TipsLocker.Unlock()
				}
			}
		}
		return nil
	})
	logs.Log.Infof("Loaded tips: %v\n", len(tips))
}

func addTip (tx *transaction.FastTX) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()
	tips = append(tips, Tip{tx.Value, tx})
}

// TODO: remove tips randomly if more than X tips in memory/DB?
func removeTip (tx *transaction.FastTX) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()
	b := tips[:0]
	for _, x := range tips {
		if !bytes.Equal(x.TX.Hash, tx.Hash) {
			b = append(b, x)
		}
	}
	tips = b
}

func updateTipsOnNewTransaction (tx *transaction.FastTX, txn *badger.Txn) error {
	key := db.GetByteKey(tx.Hash, db.KEY_APPROVEE)
	if db.CountByPrefix(db.GetByteKey(tx.Hash, db.KEY_APPROVEE)) < 1 {
		err := db.Put(db.AsKey(key, db.KEY_TIP), int64(math.Abs(float64(tx.Value))) + 1000000, nil, txn)
		if err != nil {
			return err
		}
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
	return nil
}

func getRandomTip () *transaction.FastTX {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()

	if len(tips) < 1 {
		return nil
	}

	return tips[utils.Random(0, len(tips))].TX
}