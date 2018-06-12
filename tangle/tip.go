package tangle

import (
	"bytes"
	"encoding/gob"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/transaction"
	"gitlab.com/semkodev/hercules/utils"
	"sync"
	"time"
)

type Tip struct {
	Hash      []byte
	Timestamp int
}

var Tips []*Tip
var TipsLocker = &sync.Mutex{}

func GetRandomTip() (hash []byte) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()

	if len(Tips) < 1 {
		return nil
	}

	hash = Tips[utils.Random(0, len(Tips))].Hash
	return hash
}

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

			var timestamp int
			buf := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&timestamp)
			if err == nil {
				hash, err := db.GetBytes(db.AsKey(key, db.KEY_HASH), txn)
				if err == nil {
					TipsLocker.Lock()
					Tips = append(Tips, &Tip{hash, timestamp})
					TipsLocker.Unlock()
				}
			}
		}
		return nil
	})
	logs.Log.Infof("Loaded tips: %v\n", len(Tips))
}

func startTipRemover() {
	flushTicker := time.NewTicker(tipRemoverInterval)
	for range flushTicker.C {
		logs.Log.Warning("Tips remover starting... Total tips:", len(Tips))
		var toRemove []*Tip
		TipsLocker.Lock()
		for _, tip := range Tips {
			tipAge := time.Duration(time.Now().Sub(time.Unix(int64(tip.Timestamp), 0)).Nanoseconds())
			tipAgeOK := tipAge < maxTipAge
			origKey := db.GetByteKey(tip.Hash, db.KEY_APPROVEE)
			if !tipAgeOK || db.CountByPrefix(origKey) > 0 {
				toRemove = append(toRemove, tip)
			}
		}
		TipsLocker.Unlock()
		logs.Log.Warning("Tips to remove:", len(toRemove))
		for _, tip := range toRemove {
			err := db.Remove(db.GetByteKey(tip.Hash, db.KEY_TIP), nil)
			if err == nil {
				removeTip(tip.Hash)
			}
		}
	}
}

func addTip(hash []byte, value int) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()
	if findTip(hash) >= 0 {
		return
	}

	Tips = append(Tips, &Tip{hash, value})
}

func removeTip(hash []byte) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()

	var which = findTip(hash)
	if which > -1 {
		if which >= len(Tips)-1 {
			Tips = Tips[0:which]
		} else {
			Tips = append(Tips[0:which], Tips[which+1:]...)
		}
	}
}

func findTip(hash []byte) int {
	for i, tip := range Tips {
		if bytes.Equal(hash, tip.Hash) {
			return i
		}
	}
	return -1
}

func updateTipsOnNewTransaction(tx *transaction.FastTX, txn *badger.Txn) error {
	key := db.GetByteKey(tx.Hash, db.KEY_APPROVEE)
	tipAge := time.Duration(time.Now().Sub(time.Unix(int64(tx.Timestamp), 0)).Nanoseconds())

	if tipAge < maxTipAge && db.CountByPrefix(key) < 1 {
		err := db.Put(db.AsKey(key, db.KEY_TIP), tx.Timestamp, nil, txn)
		if err != nil {
			return err
		}
		addTip(tx.Hash, tx.Timestamp)
	}

	err := db.Remove(db.GetByteKey(tx.TrunkTransaction, db.KEY_TIP), txn)
	if err == nil {
		removeTip(tx.TrunkTransaction)
	}
	err = db.Remove(db.GetByteKey(tx.BranchTransaction, db.KEY_TIP), txn)
	if err == nil {
		removeTip(tx.BranchTransaction)
	}
	return nil
}

func getRandomTip() (hash []byte, txBytes []byte) {
	TipsLocker.Lock()
	defer TipsLocker.Unlock()

	if len(Tips) < 1 {
		return nil, nil
	}

	hash = Tips[utils.Random(0, len(Tips))].Hash
	txBytes, err := db.GetBytes(db.GetByteKey(hash, db.KEY_BYTES), nil)
	if err != nil {
		return nil, nil
	}
	return hash, txBytes
}
