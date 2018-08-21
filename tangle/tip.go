package tangle

import (
	"bytes"
	"encoding/gob"
	"sync"
	"time"

	"../db"
	"../logs"
	"../transaction"
	"../utils"
)

type Tip struct {
	Hash      []byte
	Timestamp int
}

var Tips []*Tip
var TipsLock = &sync.Mutex{}

func GetRandomTip() (hash []byte) {
	TipsLock.Lock()
	defer TipsLock.Unlock()

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
	db.Singleton.View(func(tx db.Transaction) error {
		return tx.ForPrefix([]byte{db.KEY_TIP}, true, func(key, value []byte) (bool, error) {
			var timestamp int
			if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&timestamp); err != nil {
				return true, nil
			}

			hash, err := tx.GetBytes(db.AsKey(key, db.KEY_HASH))
			if err != nil {
				return true, nil
			}

			TipsLock.Lock()
			Tips = append(Tips, &Tip{hash, timestamp})
			TipsLock.Unlock()

			return true, nil
		})
	})
	logs.Log.Infof("Loaded tips: %v\n", len(Tips))
}

func startTipRemover() {
	flushTicker := time.NewTicker(tipRemoverInterval)
	for range flushTicker.C {
		logs.Log.Warning("Tips remover starting... Total tips:", len(Tips))
		var toRemove []*Tip
		TipsLock.Lock()
		for _, tip := range Tips {
			tipAge := time.Duration(time.Now().Sub(time.Unix(int64(tip.Timestamp), 0)).Nanoseconds())
			tipAgeOK := tipAge < maxTipAge
			origKey := db.GetByteKey(tip.Hash, db.KEY_APPROVEE)
			if !tipAgeOK || db.Singleton.CountPrefix(origKey) > 0 {
				toRemove = append(toRemove, tip)
			}
		}
		TipsLock.Unlock()
		logs.Log.Warning("Tips to remove:", len(toRemove))
		for _, tip := range toRemove {
			err := db.Singleton.Remove(db.GetByteKey(tip.Hash, db.KEY_TIP))
			if err == nil {
				removeTip(tip.Hash)
			}
		}
	}
}

func addTip(hash []byte, value int) {
	TipsLock.Lock()
	defer TipsLock.Unlock()
	if findTip(hash) >= 0 {
		return
	}

	Tips = append(Tips, &Tip{hash, value})
}

func removeTip(hash []byte) {
	TipsLock.Lock()
	defer TipsLock.Unlock()

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

func updateTipsOnNewTransaction(t *transaction.FastTX, tx db.Transaction) error {
	key := db.GetByteKey(t.Hash, db.KEY_APPROVEE)
	tipAge := time.Duration(time.Now().Sub(time.Unix(int64(t.Timestamp), 0)).Nanoseconds())

	if tipAge < maxTipAge && db.Singleton.CountPrefix(key) < 1 {
		err := db.Singleton.Put(db.AsKey(key, db.KEY_TIP), t.Timestamp, nil)
		if err != nil {
			return err
		}
		addTip(t.Hash, t.Timestamp)
	}

	err := db.Singleton.Remove(db.GetByteKey(t.TrunkTransaction, db.KEY_TIP))
	if err == nil {
		removeTip(t.TrunkTransaction)
	}
	err = db.Singleton.Remove(db.GetByteKey(t.BranchTransaction, db.KEY_TIP))
	if err == nil {
		removeTip(t.BranchTransaction)
	}
	return nil
}

func getRandomTip() (hash []byte, txBytes []byte) {
	TipsLock.Lock()
	defer TipsLock.Unlock()

	if len(Tips) < 1 {
		return nil, nil
	}

	hash = Tips[utils.Random(0, len(Tips))].Hash
	txBytes, err := db.Singleton.GetBytes(db.GetByteKey(hash, db.KEY_BYTES))
	if err != nil {
		return nil, nil
	}
	return hash, txBytes
}
