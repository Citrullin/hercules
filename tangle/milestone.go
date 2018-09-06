package tangle

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"../convert"
	"../db"
	"../db/coding"
	"../logs"
	"../transaction"
)

const COO_ADDRESS = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
const COO_ADDRESS2 = "999999999999999999999999999999999999999999999999999999999999999999999999999999999"

// TODO: for full-nodes this interval could be decreased for a little faster confirmations..?
const milestoneCheckInterval = time.Duration(10) * time.Second
const totalMilestoneCheckInterval = time.Duration(30) * time.Minute

var lastMilestoneCheck = time.Now()

type Milestone struct {
	TX    *transaction.FastTX
	Index int
}

type PendingMilestone struct {
	Key         []byte
	TX2BytesKey []byte
}
type PendingMilestoneQueue chan *PendingMilestone

var COO_ADDRESS_BYTES = convert.TrytesToBytes(COO_ADDRESS)[:49]
var COO_ADDRESS2_BYTES = convert.TrytesToBytes(COO_ADDRESS2)[:49]

var pendingMilestoneQueue PendingMilestoneQueue
var LatestMilestone Milestone
var LatestMilestoneLock = &sync.RWMutex{}

// TODO: remove this? Or add an API interface?
func LoadMissingMilestonesFromFile(path string) error {
	logs.Log.Info("Loading missing...")
	total := 0
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			logs.Log.Fatalf("read file line error: %v", err)
			return err
		}
		line = strings.TrimSpace(line)
		hash := convert.TrytesToBytes(line)
		if len(hash) < 49 {
			continue
		}
		hash = hash[:49]
		has, err := requestIfMissing(hash, "")
		if err == nil {
			if !has {
				//logs.Log.Warning("MISSING", line)
				total++
			} else {
				key := db.GetByteKey(hash, db.KEY_MILESTONE)
				if !db.Singleton.HasKey(key) {
					bits, err := db.Singleton.GetBytes(db.AsKey(key, db.KEY_BYTES))
					if err != nil {
						logs.Log.Error("Couldn't get milestonetx bytes", err)
						continue
					}
					trits := convert.BytesToTrits(bits)[:8019]
					tx := transaction.TritsToTX(&trits, bits)
					trunkBytesKey := db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES)
					err = db.Singleton.PutBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING), trunkBytesKey)
					pendingMilestone := &PendingMilestone{key, trunkBytesKey}
					logs.Log.Debugf("Added missing milestone: %v", convert.BytesToTrytes(tx.Hash)[:81])
					addPendingMilestoneToQueue(pendingMilestone)
				}
			}
		} else {
			logs.Log.Error("ERR", err)
		}
	}
	logs.Log.Info("Loaded missing", total)

	return nil
}

func milestoneOnLoad() {
	logs.Log.Info("Loading milestones")
	pendingMilestoneQueue = make(PendingMilestoneQueue, maxQueueSize)

	loadLatestMilestone()

	go startMilestoneChecker()
}

func loadLatestMilestone() {
	logs.Log.Infof("Loading latest milestone...")
	db.Singleton.View(func(tx db.Transaction) error {
		latest := 0
		LatestMilestone = Milestone{tipFastTX, latest}

		return coding.ForPrefixInt(tx, []byte{db.KEY_MILESTONE}, true, func(key []byte, ms int) (bool, error) {
			if ms <= latest {
				return true, nil
			}

			key = db.AsKey(key, db.KEY_BYTES)
			txBytes, err := tx.GetBytes(key)
			if err != nil {
				return true, nil
			}

			trits := convert.BytesToTrits(txBytes)[:8019]
			tx := transaction.TritsToTX(&trits, txBytes)
			LatestMilestoneLock.Lock()
			LatestMilestone = Milestone{tx, ms}
			LatestMilestoneLock.Unlock()
			latest = ms

			return true, nil
		})
	})
	logs.Log.Infof("Loaded latest milestone: %v", LatestMilestone.Index)
}

func getLatestMilestone() *Milestone {
	LatestMilestoneLock.RLock()
	defer LatestMilestoneLock.RUnlock()
	return &LatestMilestone
}

func checkIsLatestMilestone(index int, tx *transaction.FastTX) bool {
	milestone := getLatestMilestone()
	if milestone.Index < index {
		// Add milestone hash:
		trits := convert.BytesToTrits(tx.Bytes)[:8019]
		tx = transaction.TritsToTX(&trits, tx.Bytes)
		LatestMilestoneLock.Lock()
		defer LatestMilestoneLock.Unlock()
		LatestMilestone = Milestone{tx, index}
		logs.Log.Infof("Latest milestone changed to: %v", index)
		return true
	}
	return false
}

func addPendingMilestoneToQueue(pendingMilestone *PendingMilestone) {
	go func() {
		time.Sleep(time.Second * time.Duration(2))
		pendingMilestoneQueue <- pendingMilestone
	}()
}

/*
Runs checking of pending milestones.
*/
func startMilestoneChecker() {
	total := 0
	db.Singleton.Lock()
	db.Singleton.Unlock()
	var pairs []PendingMilestone
	db.Singleton.View(func(tx db.Transaction) error {
		return tx.ForPrefix([]byte{db.KEY_EVENT_MILESTONE_PENDING}, true, func(key, value []byte) (bool, error) {
			k := make([]byte, len(key))
			v := make([]byte, len(value))
			copy(k, key)
			copy(v, value)
			pairs = append(pairs, PendingMilestone{k, v})
			return true, nil
		})
	})
	for _, pair := range pairs {
		db.Singleton.Update(func(tx db.Transaction) (e error) {
			defer func() {
				if err := recover(); err != nil {
					e = errors.New("Failed milestone check!")
				}
			}()
			total += preCheckMilestone(pair.Key, pair.TX2BytesKey, tx)
			return nil
		})
	}
	pairs = nil
	lastMilestoneCheck = time.Now()

	go checkMilestones()
}

func checkMilestones() {
	for {
		for len(pendingMilestoneQueue) > 0 {
			incomingMilestone(<-pendingMilestoneQueue)
		}

		time.Sleep(milestoneCheckInterval)
		if time.Now().Sub(lastMilestoneCheck) > totalMilestoneCheckInterval {
			go startMilestoneChecker()
			break
		}
	}
}

func incomingMilestone(pendingMilestone *PendingMilestone) {
	db.Singleton.Update(func(tx db.Transaction) (e error) {
		defer func() {
			if err := recover(); err != nil {
				e = errors.New("Failed queue milestone check!")
			}
		}()
		key := db.AsKey(pendingMilestone.Key, db.KEY_EVENT_MILESTONE_PENDING)
		TX2BytesKey := pendingMilestone.TX2BytesKey
		if TX2BytesKey == nil {
			relation, err := tx.GetBytes(db.AsKey(key, db.KEY_RELATION))
			if err != nil {
				// The 0-index milestone TX relations doesn't exist.
				// Clearly an error here!
				tx.Remove(key)
				logs.Log.Panicf("A milestone has disappeared!")
				panic("A milestone has disappeared!")
			}
			TX2BytesKey = db.AsKey(relation[:16], db.KEY_HASH)
		}
		preCheckMilestone(key, TX2BytesKey, tx)
		return nil
	})
}

func preCheckMilestone(key []byte, TX2BytesKey []byte, tx db.Transaction) int {
	var txBytesKey = db.AsKey(key, db.KEY_BYTES)
	// 2. Check if 1-index TX already exists
	tx2Bytes, err := tx.GetBytes(TX2BytesKey)
	if err != nil {
		err := tx.PutBytes(db.AsKey(TX2BytesKey, db.KEY_EVENT_MILESTONE_PAIR_PENDING), key)
		if err != nil {
			logs.Log.Errorf("Could not add pending milestone pair: %v", err)
			panic(err)
		}
	}

	// 3. Check that the 0-index TX also exists.
	txBytes, err := tx.GetBytes(txBytesKey)
	if err != nil {
		// The 0-index milestone TX doesn't exist.
		// Clearly an error here!
		// db.Remove(key, txn)
		addPendingMilestoneToQueue(&PendingMilestone{key, TX2BytesKey})
		logs.Log.Panicf("A milestone has disappeared: %v", txBytesKey)
		panic("A milestone has disappeared!")
	}

	// Get TX objects and validate
	trits := convert.BytesToTrits(txBytes)[:8019]
	t := transaction.TritsToFastTX(&trits, txBytes)

	trits2 := convert.BytesToTrits(tx2Bytes)[:8019]
	t2 := transaction.TritsToFastTX(&trits2, tx2Bytes)

	if checkMilestone(txBytesKey, t, t2, trits2, tx) {
		return 1
	}

	return 0
}

func checkMilestone(key []byte, t *transaction.FastTX, t2 *transaction.FastTX, trits []int, tx db.Transaction) bool {
	key = db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING)
	discardMilestone := func() {
		logs.Log.Error("Discarding", convert.BytesToTrytes(t.Bundle)[:81])
		err := tx.Remove(key)
		if err != nil {
			logs.Log.Errorf("Could not remove pending milestone: %v", err)
			panic(err)
		}
		panic("PANIC")
	}

	// Verify correct bundle structure:
	if !bytes.Equal(t2.Address, COO_ADDRESS2_BYTES) ||
		!bytes.Equal(t2.TrunkTransaction, t.BranchTransaction) ||
		!bytes.Equal(t2.Bundle, t.Bundle) {
		logs.Log.Warning("Milestone bundle verification failed for:\n ", convert.BytesToTrytes(t.Bundle)[:81])
		discardMilestone()
		return false
	}

	// Verify milestone signature and get the index:
	milestoneIndex := getMilestoneIndex(t, trits)
	if milestoneIndex < 0 {
		logs.Log.Warning("Milestone signature verification failed for: ", convert.BytesToTrytes(t.Bundle)[:81])
		discardMilestone()
		return false
	}

	// Save milestone (remove pending) and update latest:
	err := tx.Remove(key)
	if err != nil {
		logs.Log.Errorf("Could not remove pending milestone: %v", err)
		panic(err)
	}
	err = coding.PutInt(tx, db.AsKey(key, db.KEY_MILESTONE), milestoneIndex)
	if err != nil {
		logs.Log.Errorf("Could not save milestone: %v", err)
		panic(err)
	}
	checkIsLatestMilestone(milestoneIndex, t)

	// Trigger confirmations
	err = addPendingConfirmation(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), int64(t.Timestamp), tx)
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
func getMilestoneIndex(tx *transaction.FastTX, trits []int) int {
	milestoneIndex := int(convert.TritsToInt(convert.BytesToTrits(tx.ObsoleteTag[:5])).Uint64())
	trunkTransactionTrits := convert.BytesToTrits(tx.TrunkTransaction)[:243]
	normalized := transaction.NormalizedBundle(trunkTransactionTrits)[:transaction.NUMBER_OF_FRAGMENT_CHUNKS]
	digests := transaction.Digest(normalized, tx.SignatureMessageFragment, 0, 0, false)
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
	return bytes.Equal(tx.Address, COO_ADDRESS_BYTES) && tx.Value == 0
}

func isMaybeMilestonePair(tx *transaction.FastTX) bool {
	return bytes.Equal(tx.Address, COO_ADDRESS2_BYTES) && tx.Value == 0
}

func isMaybeMilestonePart(tx *transaction.FastTX) bool {
	return tx.Value == 0 && (bytes.Equal(tx.Address, COO_ADDRESS_BYTES) || bytes.Equal(tx.Address, COO_ADDRESS2_BYTES))
}

/**
Returns the (hash) key for a specific milestone.
if acceptNearest is set, the nearest, more recent milestone is returned, if the other is not found.
*/
func GetMilestoneKeyByIndex(index int, acceptNearest bool) []byte {
	var milestoneKey []byte
	currentIndex := LatestMilestone.Index + 1

	db.Singleton.View(func(tx db.Transaction) error {
		return coding.ForPrefixInt(tx, []byte{db.KEY_MILESTONE}, true, func(key []byte, ms int) (bool, error) {
			if ms == index {
				milestoneKey = db.AsKey(key, db.KEY_HASH)
				return false, nil
			} else if acceptNearest && ms < currentIndex {
				milestoneKey = db.AsKey(key, db.KEY_HASH)
			}
			return true, nil
		})
	})

	return milestoneKey
}
