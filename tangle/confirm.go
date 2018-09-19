package tangle

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/db/coding"
	"gitlab.com/semkodev/hercules/db/ns"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/snapshot"
	"gitlab.com/semkodev/hercules/transaction"
	"github.com/pkg/errors"
)

const (
	UNKNOWN_CHECK_INTERVAL = time.Duration(60) * time.Second
)

var (
	confirmQueue                    = make(chan *PendingConfirmation, maxQueueSize)
	confirmQueueWaitGroup           = &sync.WaitGroup{}
	confirmQueueQuit                = make(chan struct{})
	removeOrphanedPendingTicker     *time.Ticker
	removeOrphanedPendingWaitGroup  = &sync.WaitGroup{}
	removeOrphanedPendingTickerQuit = make(chan struct{})
)

type PendingConfirmation struct {
	key       []byte
	timestamp int64
}

func confirmOnLoad() {

	loadPendingConfirmations()
	logs.Log.Infof("Loaded %v pending confirmations", len(confirmQueue))

	logs.Log.Info("Starting confirmation thread")
	go startUnknownVerificationThread()
	go startConfirmThread()
}

func loadPendingConfirmations() {
	db.Singleton.View(func(dbTx db.Transaction) error {
		coding.ForPrefixInt64(dbTx, ns.Prefix(ns.NamespaceEventConfirmationPending), true, func(key []byte, timestamp int64) (bool, error) {
			confirmQueue <- &PendingConfirmation{
				ns.Key(key, ns.NamespaceEventConfirmationPending),
				timestamp,
			}
			return true, nil
		})
		return nil
	})
}

func startConfirmThread() {
	confirmQueueWaitGroup.Add(1)
	defer confirmQueueWaitGroup.Done()

	for {
		select {
		case <-confirmQueueQuit:
			return

		case pendingConfirmation := <-confirmQueue:
			if ended {
				break
			}
			confirmed := false
			retryConfirm := true

			err := db.Singleton.Update(func(dbTx db.Transaction) (err error) {
				confirmed, retryConfirm, err = confirm(pendingConfirmation.key, dbTx)
				if !retryConfirm {
					dbTx.Remove(ns.Key(pendingConfirmation.key, ns.NamespaceEventConfirmationPending))
				}
				return err
			})
			if err != nil {
				logs.Log.Errorf("Error during confirmation: %v", err)
			}

			if retryConfirm {
				confirmQueue <- pendingConfirmation
			}

			if confirmed {
				totalConfirmations++
			}
		}
	}
}

func startUnknownVerificationThread() {
	removeOrphanedPendingWaitGroup.Add(1)
	defer removeOrphanedPendingWaitGroup.Done()

	removeOrphanedPendingTicker = time.NewTicker(UNKNOWN_CHECK_INTERVAL)
	for {
		select {
		case <-removeOrphanedPendingTickerQuit:
			return

		case <-removeOrphanedPendingTicker.C:
			if ended {
				break
			}
			db.Singleton.View(func(dbTx db.Transaction) error {
				var toRemove [][]byte
				ns.ForNamespace(dbTx, ns.NamespacePendingConfirmed, false, func(key, _ []byte) (bool, error) {
					if dbTx.HasKey(ns.Key(key, ns.NamespaceHash)) {
						k := make([]byte, len(key))
						copy(k, key)
						toRemove = append(toRemove, k)
					}
					return true, nil
				})

				for _, key := range toRemove {
					logs.Log.Debug("Removing orphaned pending confirmed key", key)
					err := db.Singleton.Update(func(dbTx db.Transaction) error {
						if err := dbTx.Remove(key); err != nil {
							return err
						}
						return confirmChild(ns.Key(key, ns.NamespaceHash), dbTx)
					})
					if err != nil {
						return err
					}
				}
				return nil
			})
		}
	}
}

func confirm(key []byte, dbTx db.Transaction) (newlyConfirmed bool, retryConfirm bool, err error) {

	if dbTx.HasKey(ns.Key(key, ns.NamespaceConfirmed)) {
		// Already confirmed
		return false, false, nil
	}

	data, err := dbTx.GetBytes(ns.Key(key, ns.NamespaceBytes))
	if err != nil {
		// Probably the tx is not yet committed to the database. Simply retry.
		return false, true, nil
	}

	trits := convert.BytesToTrits(data)[:8019]
	tx := transaction.TritsToFastTX(&trits, data)

	if dbTx.HasKey(ns.Key(key, ns.NamespaceEventTrimPending)) && !isMaybeMilestonePart(tx) {
		return false, false, fmt.Errorf("TX behind snapshot horizon, skipping (%v vs %v). Possible DB inconsistency! TX: %v",
			tx.Timestamp,
			snapshot.GetSnapshotTimestamp(dbTx),
			convert.BytesToTrytes(tx.Hash))
	}

	// TODO: This part should be atomic with all the value and confirmes and childs?
	//		 => If there is a DB error, revert DB commit
	err = coding.PutInt64(dbTx, ns.Key(key, ns.NamespaceConfirmed), int64(tx.Timestamp))
	if err != nil {
		return false, true, errors.New("Could not save confirmation status!")
	}

	if tx.Value != 0 {
		_, err := coding.IncrementInt64By(dbTx, ns.AddressKey(tx.Address, ns.NamespaceBalance), tx.Value, false)
		if err != nil {
			return false, true, errors.New("Could not update account balance!")
		}
		if tx.Value < 0 {
			err := coding.PutBool(dbTx, ns.AddressKey(tx.Address, ns.NamespaceSpent), true)
			if err != nil {
				return false, true, errors.New("Could not update account spent status!")
			}
		}
	}

	err = confirmChild(ns.HashKey(tx.TrunkTransaction, ns.NamespaceHash), dbTx)
	if err != nil {
		return false, true, err
	}
	err = confirmChild(ns.HashKey(tx.BranchTransaction, ns.NamespaceHash), dbTx)
	if err != nil {
		return false, true, err
	}
	return true, false, nil
}

func confirmChild(key []byte, dbTx db.Transaction) error {
	if bytes.Equal(key, tipHashKey) {
		// Doesn't need confirmation
		return nil
	}
	if dbTx.HasKey(ns.Key(key, ns.NamespaceConfirmed)) {
		// Already confirmed
		return nil
	}

	keyPending := ns.Key(key, ns.NamespaceEventConfirmationPending)
	if dbTx.HasKey(keyPending) {
		// Confirmation is already pending
		return nil
	}

	timestamp, err := coding.GetInt64(dbTx, ns.Key(key, ns.NamespaceTimestamp))
	if err == nil {
		err = addPendingConfirmation(keyPending, timestamp, dbTx)
		if err != nil {
			return fmt.Errorf("Could not save child confirm status: %v", err)
		}
	} else if !dbTx.HasKey(ns.Key(key, ns.NamespaceEdge)) && dbTx.HasKey(ns.Key(key, ns.NamespacePendingHash)) {
		err = coding.PutInt64(dbTx, ns.Key(key, ns.NamespacePendingConfirmed), time.Now().Unix())
		if err != nil {
			return fmt.Errorf("Could not save child pending confirm status: %v", err)
		}
	}
	return nil
}

func addPendingConfirmation(key []byte, timestamp int64, dbTx db.Transaction) error {
	err := coding.PutInt64(dbTx, ns.Key(key, ns.NamespaceEventConfirmationPending), timestamp)
	if err == nil {
		// TODO: find a way to add to the queue AFTER the tx has been committed.
		confirmQueue <- &PendingConfirmation{key, timestamp}
	}
	return err
}

/*
func hasConfirmInProgress(key []byte) bool {
	confirmsInProgressLock.RLock()
	defer confirmsInProgressLock.RUnlock()

	_, ok := confirmsInProgress[string(key)]
	return ok
}

func addConfirmInProgress(key []byte) bool {
	if hasConfirmInProgress(key) {
		return false
	}

	confirmsInProgressLock.Lock()
	defer confirmsInProgressLock.Unlock()

	confirmsInProgress[string(key)] = true
	return true
}

func removeConfirmInProgress(key []byte) {
	confirmsInProgressLock.Lock()
	defer confirmsInProgressLock.Unlock()
	k := string(key)
	_, ok := confirmsInProgress[k]
	if ok {
		delete(confirmsInProgress, k)
	}
}
*/

func reapplyConfirmed() {
	logs.Log.Debug("Reapplying confirmed TXs to balances")
	db.Singleton.View(func(dbTx db.Transaction) error {
		x := 0
		return ns.ForNamespace(dbTx, ns.NamespaceConfirmed, false, func(key, _ []byte) (bool, error) {
			txBytes, _ := dbTx.GetBytes(ns.Key(key, ns.NamespaceBytes))
			trits := convert.BytesToTrits(txBytes)[:8019]
			tx := transaction.TritsToFastTX(&trits, txBytes)
			if tx.Value != 0 {
				err := db.Singleton.Update(func(dbTx db.Transaction) error {
					_, err := coding.IncrementInt64By(dbTx, ns.AddressKey(tx.Address, ns.NamespaceBalance), tx.Value, false)
					if err != nil {
						logs.Log.Errorf("Could not update account balance: %v", err)
						return errors.New("Could not update account balance!")
					}
					if tx.Value < 0 {
						err := coding.PutBool(dbTx, ns.AddressKey(tx.Address, ns.NamespaceSpent), true)
						if err != nil {
							logs.Log.Errorf("Could not update account spent status: %v", err)
							return errors.New("Could not update account spent status!")
						}
					}
					return nil
				})
				if err != nil {
					logs.Log.Errorf("Could not apply tx Value: %v", err)
					return false, errors.New("Could not apply Tx value!")
				}
			}
			x = x + 1
			if x%10000 == 0 {
				logs.Log.Debug("Progress", x)
			}
			return true, nil
		})
	})
}
