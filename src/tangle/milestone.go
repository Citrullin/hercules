package tangle

import (
	"transaction"
	"bytes"
	"convert"
	"db"
	"github.com/dgraph-io/badger"
	"log"
	"encoding/gob"
	"sync"
	"time"
)

const MILESTONE_CHECK_INTERVAL = time.Duration(15) * time.Second
const COO_ADDRESS = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
const COO_ADDRESS2 = "999999999999999999999999999999999999999999999999999999999999999999999999999999999"

type Milestone struct {
	TX *transaction.FastTX
	Index int
}

var COO_ADDRESS_BYTES = convert.TrytesToBytes(COO_ADDRESS)[:49]
var COO_ADDRESS2_BYTES = convert.TrytesToBytes(COO_ADDRESS2)[:49]

var milestones map[byte]*Milestone
var MilestoneLocker = &sync.Mutex{}

func milestoneOnLoad() {
	// TODO: milestone from snapshot?
	milestones = make(map[byte]*Milestone)

	loadLatestMilestone(db.KEY_MILESTONE)
	loadLatestMilestone(db.KEY_SOLID_MILESTONE)

	go startSolidMilestoneChecker()
	go startMilestoneChecker()
}

func loadLatestMilestone (dbKey byte) {
	log.Printf("Loading latest %v...\n", milestoneType(dbKey))
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
	log.Printf("    ---> %v\n", milestones[dbKey].Index)
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
		log.Printf("Latest %v changed to: %v \n", milestoneType(dbKey), index)
		return true
	}
	return false
}

func startSolidMilestoneChecker () {
	// TODO: implement and run periodic milestone checker
	// checkIsLatestMilestone
}

func startMilestoneChecker() {
	total := 0
	db.Locker.Lock()
	_ = db.DB.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_EVENT_MILESTONE_PENDING}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			value, _ := item.Value()
			var txHashBytes = db.AsKey(key, db.KEY_BYTES)
			var tx2HashBytes = value

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
					tx2 := transaction.TritsToFastTX(&trits2, tx2Bytes)

					if checkMilestone(key, tx, tx2, trits2, txn) {
						total++
					}
				} else {
					// The 0-index milestone TX doesn't exist.
					// Clearly an error here!
					db.Remove(key, txn)
					panic("PANIC: A milestone has disappeared!")
				}
			}
		}
		return nil
	})
	db.Locker.Unlock()
	time.Sleep(MILESTONE_CHECK_INTERVAL)
	go startMilestoneChecker()
}

func checkMilestone (key []byte, tx *transaction.FastTX, tx2 *transaction.FastTX, trits []int, txn *badger.Txn) bool {
	if !bytes.Equal(tx2.Address, COO_ADDRESS2_BYTES) ||
		!bytes.Equal(tx2.TrunkTransaction, tx.BranchTransaction) ||
		!bytes.Equal(tx2.Bundle, tx.Bundle) {
		log.Println(
			"Milestone bundle verification failed for:\n ",
			convert.BytesToTrytes(tx.Hash),
			convert.BytesToTrytes(tx2.Hash),
			convert.BytesToTrytes(tx2.Address),
			convert.BytesToTrytes(COO_ADDRESS2_BYTES),
			convert.BytesToTrytes(tx2.TrunkTransaction),
			convert.BytesToTrytes(tx.BranchTransaction),
			convert.BytesToTrytes(tx2.Bundle),
			convert.BytesToTrytes(tx.Bundle))
		return false
	}

	// Verify milestone signature and get the index:
	milestoneIndex := getMilestoneIndex(tx, trits)
	if milestoneIndex < 0 {
		log.Println("Milestone signature verification failed for: ", convert.BytesToTrytes(tx.Hash))
		return false
	}

	db.Remove(db.GetByteKey(tx.Hash, db.KEY_EVENT_MILESTONE_PENDING), txn)
	// Save milestone and update latest:
	db.Put(db.GetByteKey(tx.Hash, db.KEY_MILESTONE), milestoneIndex, nil, txn)
	checkIsLatestMilestone(milestoneIndex, tx, db.KEY_MILESTONE)

	// TODO: async confirm ?
	// Trigger confirmation cascade:
	db.Put(db.GetByteKey(tx.Hash, db.KEY_EVENT_CONFIRMATION_PENDING), "", nil, txn)
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
