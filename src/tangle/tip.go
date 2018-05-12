package tangle

import (
	"transaction"
	"github.com/dgraph-io/badger"
	"db"
	"bytes"
	"encoding/gob"
	"sync"
	"utils"
	"logs"
	"time"
)

type Tip struct {
	Hash []byte
	Value int
}

var tips []*Tip
var TipsLocker = &sync.Mutex{}

func tipOnLoad() {
	loadTips()
	go startTipRemover()
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

			var value int
			buf := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&value)
			if err == nil {
				key := db.AsKey(key, db.KEY_BYTES)
				TipsLocker.Lock()
				tips = append(tips, &Tip{key,value})
				TipsLocker.Unlock()
			}
		}
		return nil
	})
	logs.Log.Infof("Loaded tips: %v\n", len(tips))
}

func startTipRemover () {
	flushTicker := time.NewTicker(tipRemoverInterval)
	for range flushTicker.C {
		logs.Log.Warning("Tips remover starting...")
		_ = db.DB.Update(func(txn *badger.Txn) error {
			var toRemove []*Tip
			TipsLocker.Lock()
			for _, tip := range tips {
				tipAge := time.Duration(time.Now().Sub(time.Unix(int64(tip.Value), 0)).Nanoseconds())
				tipAgeOK := tipAge < maxTipAge
				origKey := db.GetByteKey(tip.Hash, db.KEY_APPROVEE)
				if !tipAgeOK || db.CountByPrefix(origKey) > 0 {
					toRemove = append(toRemove, tip)
				}
			}
			TipsLocker.Unlock()
			logs.Log.Warning("Tips to remove:", len(toRemove))
			for _, tip := range toRemove {
				db.Remove(db.GetByteKey(tip.Hash, db.KEY_TIP), txn)
				removeTip(tip.Hash)
			}
			return nil
		})
	}
}

func addTip (hash []byte, value int) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()
	tips = append(tips, &Tip{hash, value})
}

func removeTip (hash []byte) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()
	b := tips[:0]
	for _, x := range tips {
		if !bytes.Equal(x.Hash, hash) {
			b = append(b, x)
		}
	}
	tips = b
}

func updateTipsOnNewTransaction (tx *transaction.FastTX, txn *badger.Txn) error {
	key := db.GetByteKey(tx.Hash, db.KEY_APPROVEE)
	tipAge := time.Duration(time.Now().Sub(time.Unix(int64(tx.Timestamp), 0)).Nanoseconds())
	if tipAge < maxTipAge && db.CountByPrefix(db.GetByteKey(tx.Hash, db.KEY_APPROVEE)) < 1 {
		err := db.Put(db.AsKey(key, db.KEY_TIP), tx.Timestamp, nil, txn)
		if err != nil {
			return err
		}
		addTip(tx.Hash, tx.Timestamp)
	}
	err := db.Remove(db.GetByteKey(tx.TrunkTransaction, db.KEY_TIP), txn)
	if err == nil {
		removeTip(tx.Hash)
	}
	err = db.Remove(db.GetByteKey(tx.BranchTransaction, db.KEY_TIP), txn)
	if err == nil {
		removeTip(tx.Hash)
	}
	return nil
}

func getRandomTip () (hash []byte, txBytes []byte) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()

	if len(tips) < 1 {
		return nil, nil
	}

	hash = tips[utils.Random(0, len(tips))].Hash
	txBytes, err := db.GetBytes(db.GetByteKey(hash, db.KEY_BYTES), nil)
	if err != nil { return nil, nil }
	return hash, txBytes
}