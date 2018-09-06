package tangle

import (
	"bytes"
	"encoding/gob"
	"sync"
	"time"

	"../db"
	"../logs"
	"../transaction"

	"github.com/lukechampine/randmap" // github.com/lukechampine/randmap/safe is safer, but for now we use the faster one
)

var (
	Tips     = make(map[string]int64) // []byte = Hash, int = Timestamp
	TipsLock = &sync.RWMutex{}
)

func tipOnLoad() {
	loadTips()
	go tipsRemover()
}

func loadTips() {
	TipsLock.Lock()
	defer TipsLock.Unlock()

	db.Singleton.View(func(tx db.Transaction) error {
		return tx.ForPrefix([]byte{db.KEY_TIP}, true, func(key, value []byte) (bool, error) {
			var timestamp int64
			if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&timestamp); err != nil {
				return true, nil
			}

			hash, err := tx.GetBytes(db.AsKey(key, db.KEY_HASH))
			if err != nil {
				return true, nil
			}

			Tips[string(hash)] = timestamp

			return true, nil
		})
	})
	logs.Log.Infof("Loaded tips: %v\n", len(Tips))
}

func tipsRemover() {
	tipRemoverTicker := time.NewTicker(tipRemoverInterval)
	for range tipRemoverTicker.C {
		tipsToRemove, tipsCnt := getTipsToRemove()
		logs.Log.Infof("Total tips: %v | Tips to remove: %v", tipsCnt, len(tipsToRemove))

		removeTips(tipsToRemove)
	}
}

func getTipsToRemove() (tipsToRemove []string, tipsCnt int) {
	TipsLock.RLock()
	defer TipsLock.RUnlock()

	for hash, timestamp := range Tips {
		tipAge := time.Duration(time.Now().Sub(time.Unix(timestamp, 0)))

		if tipAge >= maxTipAge {
			// Tip is too old
			tipsToRemove = append(tipsToRemove, hash)
		} else {
			origKey := db.GetByteKey([]byte(hash), db.KEY_APPROVEE)
			if db.Singleton.CountPrefix(origKey) > 0 {
				// Tip was already approved
				tipsToRemove = append(tipsToRemove, hash)
			}
		}
	}

	return tipsToRemove, len(Tips)
}

func addTip(hash string, timestamp int64) {
	TipsLock.RLock()
	if _, exists := Tips[hash]; !exists {
		TipsLock.RUnlock()
		TipsLock.Lock()
		defer TipsLock.Unlock()
		Tips[hash] = timestamp
	} else {
		TipsLock.RUnlock()
	}
}

func removeTips(tipsToRemove []string) {
	TipsLock.Lock()
	defer TipsLock.Unlock()

	for _, hash := range tipsToRemove {
		db.Singleton.Remove(db.GetByteKey([]byte(hash), db.KEY_TIP))
		delete(Tips, hash)
	}
}

func removeTip(hash []byte) {
	TipsLock.Lock()
	defer TipsLock.Unlock()

	delete(Tips, string(hash))
}

func updateTipsOnNewTransaction(t *transaction.FastTX, tx db.Transaction) error {
	key := db.GetByteKey(t.Hash, db.KEY_APPROVEE)
	tipAge := time.Duration(time.Now().Sub(time.Unix(int64(t.Timestamp), 0)))

	if tipAge < maxTipAge && db.Singleton.CountPrefix(key) < 1 {
		err := db.Singleton.Put(db.AsKey(key, db.KEY_TIP), t.Timestamp, nil)
		if err != nil {
			return err
		}
		addTip(string(t.Hash), int64(t.Timestamp))
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
	TipsLock.RLock()

	if len(Tips) < 1 {
		TipsLock.RUnlock()
		return nil, nil
	}

	hashStr := randmap.FastKey(Tips).(string)
	TipsLock.RUnlock()

	hash = []byte(hashStr)
	txBytes, err := db.Singleton.GetBytes(db.GetByteKey(hash, db.KEY_BYTES))
	if err != nil {
		return nil, nil
	}
	return hash, txBytes
}
