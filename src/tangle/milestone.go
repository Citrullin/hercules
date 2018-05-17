package tangle

import (
	"transaction"
	"bytes"
	"convert"
	"db"
	"errors"
	"github.com/dgraph-io/badger"
	"encoding/gob"
	"sync"
	"logs"
	"time"
)

const COO_ADDRESS = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
const COO_ADDRESS2 = "999999999999999999999999999999999999999999999999999999999999999999999999999999999"
const milestoneCheckInterval = time.Duration(20) * time.Second

type Milestone struct {
	TX *transaction.FastTX
	Index int
}

type PendingMilestone struct {
	Key        []byte
	TX2Bytes   []byte
}
type PendingMilestoneQueue chan *PendingMilestone

var COO_ADDRESS_BYTES = convert.TrytesToBytes(COO_ADDRESS)[:49]
var COO_ADDRESS2_BYTES = convert.TrytesToBytes(COO_ADDRESS2)[:49]
var pendingMilestoneQueue PendingMilestoneQueue

var milestones map[byte]*Milestone
var MilestoneLocker = &sync.Mutex{}

func milestoneOnLoad() {
	logs.Log.Info("Loading milestones")
	// TODO: milestone from snapshot?
	milestones = make(map[byte]*Milestone)
	pendingMilestoneQueue = make(PendingMilestoneQueue, maxQueueSize)

	loadLatestMilestone(db.KEY_MILESTONE)

	//go startSolidMilestoneChecker()
	go startMilestoneChecker()
}

func loadLatestMilestone (dbKey byte) {
	logs.Log.Infof("Loading latest %v...", milestoneType(dbKey))
	_ = db.DB.View(func(txn *badger.Txn) error {
		latest := 0
		milestones[dbKey] = &Milestone{nil,latest}
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{dbKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			value, _ := item.Value()
			var ms = 0
			buf := bytes.NewBuffer(value)
			dec := gob.NewDecoder(buf)
			err := dec.Decode(&ms)
			if err == nil && ms > latest {
				key := db.AsKey(key, db.KEY_BYTES)
				txBytes, err := db.GetBytes(key, txn)
				if err == nil {
					trits := convert.BytesToTrits(txBytes)[:8019]
					tx := transaction.TritsToFastTX(&trits, txBytes)
					MilestoneLocker.Lock()
					milestones[dbKey] = &Milestone{tx,ms}
					MilestoneLocker.Unlock()
					latest = ms
				}
			}
		}
		return nil
	})
	logs.Log.Infof("Loaded latest %v: %v", milestoneType(dbKey), milestones[dbKey].Index)
}

func getLatestMilestone(dbKey byte) *Milestone {
	MilestoneLocker.Lock()
	defer MilestoneLocker.Unlock()
	return milestones[dbKey]
}

func checkIsLatestMilestone (index int, tx *transaction.FastTX, dbKey byte) bool {
	milestone := getLatestMilestone(dbKey)
	if milestone.Index < index {
		MilestoneLocker.Lock()
		defer MilestoneLocker.Unlock()
		milestones[dbKey] = &Milestone{tx,index}
		logs.Log.Infof("Latest %v changed to: %v", milestoneType(dbKey), index)
		return true
	}
	return false
}

func startSolidMilestoneChecker () {
	// TODO: implement and run periodic milestone checker? How to know when in sync?
	// checkIsLatestMilestone
}

func addPendingMilestoneToQueue (pendingMilestone *PendingMilestone) {
	go func() {
		time.Sleep(time.Second * time.Duration(2))
		pendingMilestoneQueue <- pendingMilestone
	}()
}

/*
Runs checking of pending milestones. If the
 */
func startMilestoneChecker() {
	total := 0
	db.Locker.Lock()
	var pairs []PendingMilestone
	_ = db.DB.View(func(txn *badger.Txn) (e error) {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_EVENT_MILESTONE_PENDING}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			value, _ := item.Value()
			TX2Bytes, _ := db.GetBytes(value, txn)
			pairs = append(pairs, PendingMilestone{key, TX2Bytes})
		}
		return nil
	})
	for _, pair := range pairs {
		_ = db.DB.Update(func(txn *badger.Txn) (e error) {
			defer func() {
				if err := recover(); err != nil {
					e = errors.New("Failed startup milestone check!")
				}
			}()
			total += preCheckMilestone(pair.Key, pair.TX2Bytes, txn)
			return nil
		})
	}
	pairs = nil
	db.Locker.Unlock()

	checkMilestones()
}

func checkMilestones () {
	logs.Log.Warning("checkMilestones start")
	stop := false
	for !stop {
		var pendingMilestone *PendingMilestone

		select {
		case pendingMilestone = <- pendingMilestoneQueue:
		default:
			stop = true
		}
		if stop { break }

		_ = db.DB.Update(func(txn *badger.Txn) (e error) {
			defer func() {
				if err := recover(); err != nil {
					e = errors.New("Failed queue milestone check!")
				}
			}()
			key := pendingMilestone.Key
			tx2HashBytes := pendingMilestone.TX2Bytes
			if tx2HashBytes == nil {
				relation, err := db.GetBytes(db.AsKey(key, db.KEY_RELATION), txn)
				if err == nil {
					tx2HashBytes = db.AsKey(relation[:16], db.KEY_HASH)
				} else {
					// The 0-index milestone TX relations doesn't exist.
					// Clearly an error here!
					db.Remove(key, txn)
					logs.Log.Panicf("A milestone has disappeared!")
					panic("A milestone has disappeared!")
				}
			}
			preCheckMilestone(key, tx2HashBytes, txn)
			return nil
		})
	}
	logs.Log.Warning("checkMilestones finish")
	time.Sleep(milestoneCheckInterval)
	checkMilestones()
}

func preCheckMilestone(key []byte, tx2HashBytes []byte, txn *badger.Txn) int {
	var txHashBytes = db.AsKey(key, db.KEY_BYTES)
	// 2. Check if 1-index TX already exists
	tx2Bytes, err := db.GetBytes(tx2HashBytes, txn)
	if err == nil {
		// 3. Check that the 0-index TX also exists.
		txBytes, err := db.GetBytes(txHashBytes, txn)
		if err == nil {

			//Get TX objects and validate
			trits := convert.BytesToTrits(txBytes)[:8019]
			tx := transaction.TritsToFastTX(&trits, txBytes)

			trits2 := convert.BytesToTrits(tx2Bytes)[:8019]
			// TODO: improvement: theoretically we do not need hash of tx2 - lighter version of obj?
			tx2 := transaction.TritsToFastTX(&trits2, tx2Bytes)

			if checkMilestone(tx, tx2, trits2, txn) {
				return 1
			}
		} else {
			// The 0-index milestone TX doesn't exist.
			// Clearly an error here!
			// db.Remove(key, txn)
			addPendingMilestoneToQueue(&PendingMilestone{key, tx2HashBytes})
			logs.Log.Panicf("A milestone has disappeared: %v", txHashBytes)
			panic("A milestone has disappeared!")
		}
	} else {
		err := db.Put(db.AsKey(tx2HashBytes, db.KEY_EVENT_MILESTONE_PAIR_PENDING), key, nil, txn)
		if err != nil {
			logs.Log.Errorf("Could not add pending milestone pair: %v", err)
			panic(err)
		}
	}
	return 0
}

func checkMilestone (tx *transaction.FastTX, tx2 *transaction.FastTX, trits []int, txn *badger.Txn) bool {
	discardMilestone := func () {
		panic("PANIC")
		err := db.Remove(db.GetByteKey(tx.Hash, db.KEY_EVENT_MILESTONE_PENDING), txn)
		if err != nil {
			logs.Log.Errorf("Could not remove pending milestone: %v", err)
			panic(err)
		}
	}

	// Verify correct bundle structure:
	if !bytes.Equal(tx2.Address, COO_ADDRESS2_BYTES) ||
		!bytes.Equal(tx2.TrunkTransaction, tx.BranchTransaction) ||
		!bytes.Equal(tx2.Bundle, tx.Bundle) {
		logs.Log.Warning(
			"Milestone bundle verification failed for:\n ",
			convert.BytesToTrytes(tx.Hash),
			convert.BytesToTrytes(tx2.Hash),
			convert.BytesToTrytes(tx2.Address),
			convert.BytesToTrytes(COO_ADDRESS2_BYTES),
			convert.BytesToTrytes(tx2.TrunkTransaction),
			convert.BytesToTrytes(tx.BranchTransaction),
			convert.BytesToTrytes(tx2.Bundle),
			convert.BytesToTrytes(tx.Bundle))
		discardMilestone()
		return false
	}

	// Verify milestone signature and get the index:
	milestoneIndex := getMilestoneIndex(tx, trits)
	if milestoneIndex < 0 {
		logs.Log.Warning("Milestone signature verification failed for: ", convert.BytesToTrytes(tx.Hash))
		discardMilestone()
		return false
	}

	// Save milestone (remove pending) and update latest:
	err := db.Remove(db.GetByteKey(tx.Hash, db.KEY_EVENT_MILESTONE_PENDING), txn)
	if err != nil {
		logs.Log.Errorf("Could not remove pending milestone: %v", err)
		panic(err)
	}
	err = db.Put(db.GetByteKey(tx.Hash, db.KEY_MILESTONE), milestoneIndex, nil, txn)
	if err != nil {
		logs.Log.Errorf("Could not save milestone: %v", err)
		panic(err)
	}
	checkIsLatestMilestone(milestoneIndex, tx, db.KEY_MILESTONE)

	// Trigger confirmations
	err = db.Put(db.GetByteKey(tx.Hash, db.KEY_EVENT_CONFIRMATION_PENDING), "", nil, txn)
	if err != nil {
		logs.Log.Errorf("Could not save pending confirmation: %v", err)
		panic(err)
	}
	return true
}

/*
Returns Milestone index if the milestone verification has been correct. Otherwise -1.
Params: tx + trits of the seconds transaction in the milestone bundle.
 */
func getMilestoneIndex (tx *transaction.FastTX, trits []int) int {
	milestoneIndex := int(convert.TritsToInt(convert.BytesToTrits(tx.ObsoleteTag[:5])))
	trunkTransactionTrits := convert.BytesToTrits(tx.TrunkTransaction)[:243]
	normalized := transaction.NormalizedBundle(trunkTransactionTrits)[:transaction.NUMBER_OF_FRAGMENT_CHUNKS]
	digests := transaction.Digest(normalized, tx.SignatureMessageFragment)
	address := transaction.Address(digests)
	merkleRoot := transaction.GetMerkleRoot(
		address,
		trits,
		0,
		milestoneIndex,
		transaction.NUMBER_OF_KEYS_IN_MILESTONE,
	)
	merkleAddress := convert.TritsToTrytes(merkleRoot)
	if merkleAddress == COO_ADDRESS {
		return milestoneIndex
	} else {
		return -1
	}
}

func isMaybeMilestone(tx *transaction.FastTX) bool {
	return bytes.Equal(tx.Address, COO_ADDRESS_BYTES)
}

func milestoneType (dbKey byte) string {
	if dbKey == db.KEY_SOLID_MILESTONE {
		return "solid milestone"
	} else if dbKey == db.KEY_MILESTONE {
		return "milestone"
	} else {
		return "Unknown"
	}
}
