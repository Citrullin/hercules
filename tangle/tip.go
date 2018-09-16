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
	Tips                 = make(map[string]time.Time) // []byte = Hash, time.Time = ReceiveTimestamp
	TipsLock             = &sync.RWMutex{}
	tipRemoverTicker     *time.Ticker
	tipRemoverWaitGroup  = &sync.WaitGroup{}
	tipRemoverTickerQuit = make(chan struct{})
)

func tipOnLoad() {
	loadTipsFromDB()
	go tipsRemover()
}

func loadTipsFromDB() {
	TipsLock.Lock()
	defer TipsLock.Unlock()

	db.Singleton.View(func(dbTx db.Transaction) error {
		return coding.ForPrefixInt64(dbTx, ns.Prefix(ns.NamespaceTip), true, func(key []byte, timestamp int64) (bool, error) {
			hash, err := dbTx.GetBytes(ns.Key(key, ns.NamespaceHash))
			if err != nil {
				return true, nil
			}

			Tips[string(hash)] = time.Unix(timestamp, 0)

			return true, nil
		})
	})
	logs.Log.Infof("Loaded tips: %v\n", len(Tips))
}

func tipsRemover() {
	tipRemoverWaitGroup.Add(1)
	defer tipRemoverWaitGroup.Done()

	executeTipsRemover()

	tipRemoverTicker = time.NewTicker(tipRemoverInterval)
	for {
		select {
		case <-tipRemoverTickerQuit:
			return

		case <-tipRemoverTicker.C:
			if ended {
				break
			}
			executeTipsRemover()
		}
	}
}

func executeTipsRemover() {
	tipsToRemove, tipsCnt := getTipsToRemove()
	logs.Log.Infof("Tips to remove: %v/%v", len(tipsToRemove), tipsCnt)

	db.Singleton.Update(func(dbTx db.Transaction) error {
		removeTips(tipsToRemove, dbTx)
		return nil
	})
}

func getTipsToRemove() (tipsToRemove []string, tipsCnt int) {
	keysToCheck := make(map[string][]byte)

	TipsLock.RLock()
	tipsCnt = len(Tips)

	for hash, timestamp := range Tips {
		tipAge := time.Duration(time.Now().Sub(timestamp))

		if tipAge >= maxTipAge {
			// Tip is too old
			tipsToRemove = append(tipsToRemove, hash)
		} else {
			keysToCheck[hash] = ns.HashKey([]byte(hash), ns.NamespaceApprovee)
		}
	}
	TipsLock.RUnlock()

	db.Singleton.View(func(dbTx db.Transaction) error {
		for hash, keyToCheck := range keysToCheck {
			if dbTx.CountPrefix(keyToCheck) > 0 {
				// Tip was already approved
				tipsToRemove = append(tipsToRemove, hash)
			}
		}
		return nil
	})

	return tipsToRemove, tipsCnt
}

func addTip(hash string, timestamp int, dbTx db.Transaction) error {

	TipsLock.RLock()
	if _, exists := Tips[hash]; !exists {
		TipsLock.RUnlock()

		timestampUnix := time.Unix(int64(timestamp), 0)
		tipAge := time.Duration(time.Now().Sub(timestampUnix))
		key := ns.HashKey([]byte(hash), ns.NamespaceApprovee)

		if tipAge < maxTipAge && dbTx.CountPrefix(key) < 1 {
			TipsLock.Lock()
			Tips[hash] = timestampUnix
			TipsLock.Unlock()

			return coding.PutInt64(dbTx, ns.Key(key, ns.NamespaceTip), int64(timestamp))
		}
	} else {
		TipsLock.RUnlock()
	}
	return nil
}

func removeTips(tipsToRemove []string, dbTx db.Transaction) {
	TipsLock.Lock()
	defer TipsLock.Unlock()

	for _, hash := range tipsToRemove {
		dbTx.Remove(ns.HashKey([]byte(hash), ns.NamespaceTip))
		delete(Tips, hash)
	}
}

func updateTipsOnNewTransaction(tx *transaction.FastTX, dbTx db.Transaction) error {
	addTip(string(tx.Hash), tx.Timestamp, dbTx)

	tipsToRemove := []string{string(tx.TrunkTransaction), string(tx.BranchTransaction)}
	removeTips(tipsToRemove, dbTx)
	return nil
}

func getRandomTip(dbTx db.Transaction) (hash []byte, txBytes []byte) {
	TipsLock.RLock()

	if len(Tips) < 1 {
		TipsLock.RUnlock()
		return nil, nil
	}

	hashStr := randmap.FastKey(Tips).(string)
	TipsLock.RUnlock()

	if dbTx == nil {
		dbTx = db.Singleton.NewTransaction(false)
		defer dbTx.Discard()
	}

	hash = []byte(hashStr)
	txBytes, err := dbTx.GetBytes(ns.HashKey(hash, ns.NamespaceBytes))
	if err != nil {
		return nil, nil
	}
	return hash, txBytes
}
