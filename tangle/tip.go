package tangle

import (
	"bytes"
	"sync"
	"time"

	"../db"
	"../db/coding"
	"../logs"
	"../transaction"
	"../utils"
)

type Tip struct {
	Hash      []byte
	Timestamp int64
}

var Tips []*Tip
var TipsLock = &sync.RWMutex{}

func GetRandomTip() (hash []byte) {
	TipsLock.RLock()
	defer TipsLock.RUnlock()

	if len(Tips) < 1 {
		return nil
	}

	hash = Tips[utils.Random(0, len(Tips))].Hash
	return hash
}

func tipOnLoad() {
	loadTips()
	go tipsRemover()
}

func loadTips() {
	db.Singleton.View(func(tx db.Transaction) error {
		return coding.ForPrefixInt64(tx, []byte{db.KEY_TIP}, true, func(key []byte, timestamp int64) (bool, error) {
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

func tipsRemover() {
	tipRemoverTicker := time.NewTicker(tipRemoverInterval)
	for range tipRemoverTicker.C {
		toRemove := getTipsToRemove()
		logs.Log.Infof("Total tips: %v | Tips to remove: %v", len(Tips), len(toRemove))

		removeTips(toRemove)
	}
}

func getTipsToRemove() []*Tip {
	var toRemove []*Tip
	TipsLock.RLock()
	defer TipsLock.RUnlock()

	for _, tip := range Tips {
		tipAge := time.Duration(time.Now().Sub(time.Unix(int64(tip.Timestamp), 0)).Nanoseconds())
		tipAgeOK := tipAge < maxTipAge
		origKey := db.GetByteKey(tip.Hash, db.KEY_APPROVEE)
		if !tipAgeOK || db.Singleton.CountPrefix(origKey) > 0 {
			toRemove = append(toRemove, tip)
		}
	}

	return toRemove
}

func addTip(hash []byte, value int64) {
	if findTip(hash) >= 0 {
		return
	}

	TipsLock.Lock()
	defer TipsLock.Unlock()
	Tips = append(Tips, &Tip{hash, value})
}

func removeTips(tipsToRemove []*Tip) {
	for _, tip := range tipsToRemove {
		err := db.Singleton.Remove(db.GetByteKey(tip.Hash, db.KEY_TIP))
		if err == nil {
			removeTip(tip.Hash)
		}
	}
}
func removeTip(hash []byte) {
	var which = findTip(hash)

	TipsLock.Lock()
	defer TipsLock.Unlock()

	if which > -1 {
		if which >= len(Tips)-1 {
			Tips = Tips[0:which]
		} else {
			Tips = append(Tips[0:which], Tips[which+1:]...)
		}
	}
}

func findTip(hash []byte) int {
	TipsLock.RLock()
	defer TipsLock.RUnlock()

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
		err := coding.PutInt64(db.Singleton, db.AsKey(key, db.KEY_TIP), int64(t.Timestamp))
		if err != nil {
			return err
		}
		addTip(t.Hash, int64(t.Timestamp))
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

	hash = Tips[utils.Random(0, len(Tips))].Hash
	TipsLock.RUnlock()

	txBytes, err := db.Singleton.GetBytes(db.GetByteKey(hash, db.KEY_BYTES))
	if err != nil {
		return nil, nil
	}
	return hash, txBytes
}
