package tangle

import (
	"sync"
	"time"

	"../db"
	"../db/coding"
	"../db/ns"
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
		return coding.ForPrefixInt64(tx, ns.Prefix(ns.NamespaceTip), true, func(key []byte, timestamp int64) (bool, error) {
			hash, err := tx.GetBytes(ns.Key(key, ns.NamespaceHash))
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
			origKey := ns.HashKey([]byte(hash), ns.NamespaceApprovee)
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
		db.Singleton.Remove(ns.HashKey([]byte(hash), ns.NamespaceTip))
		delete(Tips, hash)
	}
}

func removeTip(hash []byte) {
	TipsLock.Lock()
	defer TipsLock.Unlock()

	delete(Tips, string(hash))
}

func updateTipsOnNewTransaction(t *transaction.FastTX, tx db.Transaction) error {
	key := ns.HashKey(t.Hash, ns.NamespaceApprovee)
	tipAge := time.Duration(time.Now().Sub(time.Unix(int64(t.Timestamp), 0)))

	if tipAge < maxTipAge && db.Singleton.CountPrefix(key) < 1 {
		err := coding.PutInt64(db.Singleton, ns.Key(key, ns.NamespaceTip), int64(t.Timestamp))
		if err != nil {
			return err
		}
		addTip(string(t.Hash), int64(t.Timestamp))
	}

	err := db.Singleton.Remove(ns.HashKey(t.TrunkTransaction, ns.NamespaceTip))
	if err == nil {
		removeTip(t.TrunkTransaction)
	}
	err = db.Singleton.Remove(ns.HashKey(t.BranchTransaction, ns.NamespaceTip))
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
	txBytes, err := db.Singleton.GetBytes(ns.HashKey(hash, ns.NamespaceBytes))
	if err != nil {
		return nil, nil
	}
	return hash, txBytes
}
