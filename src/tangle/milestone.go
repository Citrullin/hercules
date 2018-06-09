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
	"os"
	"bufio"
	"io"
)

const COO_ADDRESS = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
const COO_ADDRESS2 = "999999999999999999999999999999999999999999999999999999999999999999999999999999999"
const milestoneCheckInterval = time.Duration(10) * time.Second
const totalMilestoneCheckInterval = time.Duration(30) * time.Minute

var lastMilestoneCheck = time.Now()

type Milestone struct {
	TX *transaction.FastTX
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
var MilestoneLocker = &sync.Mutex{}

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
		hash := convert.TrytesToBytes(line)[:49]
		has, err := requestIfMissing(hash, "", nil)
		if err == nil {
			if !has {
				//logs.Log.Warning("MISSING", line)
				total++
			} else {
				key := db.GetByteKey(hash, db.KEY_MILESTONE)
				if !db.Has(key, nil) {
					bits, err := db.GetBytes(db.AsKey(key, db.KEY_BYTES), nil)
					if err != nil {
						logs.Log.Error("Couldn't get milestonetx bytes", err)
						continue
					}
					trits := convert.BytesToTrits(bits)[:8019]
					tx := transaction.TritsToTX(&trits, bits)
					trunkBytesKey := db.GetByteKey(tx.TrunkTransaction, db.KEY_BYTES)
					err = db.PutBytes(db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING), trunkBytesKey, nil, nil)
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

	//go startSolidMilestoneChecker()
	go startMilestoneChecker()
}

func loadLatestMilestone () {
	logs.Log.Infof("Loading latest milestone...")
	_ = db.DB.View(func(txn *badger.Txn) error {
		latest := 0
		LatestMilestone = Milestone{tipFastTX,latest}
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_MILESTONE}
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
					tx := transaction.TritsToTX(&trits, txBytes)
					MilestoneLocker.Lock()
					LatestMilestone = Milestone{tx,ms}
					MilestoneLocker.Unlock()
					latest = ms
				}
			}
		}
		return nil
	})
	logs.Log.Infof("Loaded latest milestone: %v",  LatestMilestone.Index)
}

func getLatestMilestone() *Milestone {
	MilestoneLocker.Lock()
	defer MilestoneLocker.Unlock()
	return &LatestMilestone
}

func checkIsLatestMilestone (index int, tx *transaction.FastTX) bool {
	milestone := getLatestMilestone()
	if milestone.Index < index {
		MilestoneLocker.Lock()
		defer MilestoneLocker.Unlock()
		// Add milestone hash:
		trits := convert.BytesToTrits(tx.Bytes)[:8019]
		tx = transaction.TritsToTX(&trits, tx.Bytes)
		LatestMilestone = Milestone{tx,index}
		logs.Log.Infof("Latest milestone changed to: %v", index)
		return true
	}
	return false
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
	// TODO: if milestone is pending  for too long, remove it from the loop
	// TODO: also remove the events for milestone pairs
	total := 0
	db.Locker.Lock()
	db.Locker.Unlock()
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
			k := make([]byte, len(key))
			v := make([]byte, len(value))
			copy(k, key)
			copy(v, value)
			pairs = append(pairs, PendingMilestone{k, v})
		}
		return nil
	})
	for _, pair := range pairs {
		_ = db.DB.Update(func(txn *badger.Txn) (e error) {
			defer func() {
				if err := recover(); err != nil {
					e = errors.New("Failed milestone check!")
				}
			}()
			total += preCheckMilestone(pair.Key, pair.TX2BytesKey, txn)
			return nil
		})
	}
	pairs = nil
	lastMilestoneCheck = time.Now()

	go checkMilestones()
}

func checkMilestones () {
	for {
		stop := false
		for !stop {
			var pendingMilestone *PendingMilestone

			select {
			case pendingMilestone = <- pendingMilestoneQueue:
			default:
				stop = true
			}
			if stop { break }
			incomingMilestone(pendingMilestone)
		}
		time.Sleep(milestoneCheckInterval)
		if time.Now().Sub(lastMilestoneCheck) > totalMilestoneCheckInterval {
			go startMilestoneChecker()
			break
		}
	}
}

func incomingMilestone(pendingMilestone *PendingMilestone) {
	_ = db.DB.Update(func(txn *badger.Txn) (e error) {
		defer func() {
			if err := recover(); err != nil {
				e = errors.New("Failed queue milestone check!")
			}
		}()
		key := db.AsKey(pendingMilestone.Key, db.KEY_EVENT_MILESTONE_PENDING)
		TX2BytesKey := pendingMilestone.TX2BytesKey
		if TX2BytesKey == nil {
			relation, err := db.GetBytes(db.AsKey(key, db.KEY_RELATION), txn)
			if err == nil {
				TX2BytesKey = db.AsKey(relation[:16], db.KEY_HASH)
			} else {
				// The 0-index milestone TX relations doesn't exist.
				// Clearly an error here!
				db.Remove(key, txn)
				logs.Log.Panicf("A milestone has disappeared!")
				panic("A milestone has disappeared!")
			}
		}
		preCheckMilestone(key, TX2BytesKey, txn)
		return nil
	})
}

func preCheckMilestone(key []byte, TX2BytesKey []byte, txn *badger.Txn) int {
	var txBytesKey = db.AsKey(key, db.KEY_BYTES)
	// 2. Check if 1-index TX already exists
	tx2Bytes, err := db.GetBytes(TX2BytesKey, txn)
	if err == nil {
		// 3. Check that the 0-index TX also exists.
		txBytes, err := db.GetBytes(txBytesKey, txn)
		if err == nil {

			//Get TX objects and validate
			trits := convert.BytesToTrits(txBytes)[:8019]
			tx := transaction.TritsToFastTX(&trits, txBytes)

			trits2 := convert.BytesToTrits(tx2Bytes)[:8019]
			tx2 := transaction.TritsToFastTX(&trits2, tx2Bytes)

			if checkMilestone(txBytesKey, tx, tx2, trits2, txn) {
				return 1
			}
		} else {
			// The 0-index milestone TX doesn't exist.
			// Clearly an error here!
			// db.Remove(key, txn)
			addPendingMilestoneToQueue(&PendingMilestone{key, TX2BytesKey})
			logs.Log.Panicf("A milestone has disappeared: %v", txBytesKey)
			panic("A milestone has disappeared!")
		}
	} else {
		err := db.Put(db.AsKey(TX2BytesKey, db.KEY_EVENT_MILESTONE_PAIR_PENDING), key, nil, txn)
		if err != nil {
			logs.Log.Errorf("Could not add pending milestone pair: %v", err)
			panic(err)
		}
	}
	return 0
}

func checkMilestone (key []byte, tx *transaction.FastTX, tx2 *transaction.FastTX, trits []int, txn *badger.Txn) bool {
	key = db.AsKey(key, db.KEY_EVENT_MILESTONE_PENDING)
	discardMilestone := func () {
		logs.Log.Error("Discarding", convert.BytesToTrytes(tx.Bundle)[:81])
		err := db.Remove(key, nil)
		if err != nil {
			logs.Log.Errorf("Could not remove pending milestone: %v", err)
			panic(err)
		}
		panic("PANIC")
	}

	// Verify correct bundle structure:
	if !bytes.Equal(tx2.Address, COO_ADDRESS2_BYTES) ||
		!bytes.Equal(tx2.TrunkTransaction, tx.BranchTransaction) ||
		!bytes.Equal(tx2.Bundle, tx.Bundle) {
		logs.Log.Warning("Milestone bundle verification failed for:\n ", convert.BytesToTrytes(tx.Bundle)[:81])
		discardMilestone()
		return false
	}

	// Verify milestone signature and get the index:
	milestoneIndex := getMilestoneIndex(tx, trits)
	if milestoneIndex < 0 {
		logs.Log.Warning("Milestone signature verification failed for: ", convert.BytesToTrytes(tx.Bundle)[:81])
		discardMilestone()
		return false
	}

	// Save milestone (remove pending) and update latest:
	err := db.Remove(key, txn)
	if err != nil {
		logs.Log.Errorf("Could not remove pending milestone: %v", err)
		panic(err)
	}
	err = db.Put(db.AsKey(key, db.KEY_MILESTONE), milestoneIndex, nil, txn)
	if err != nil {
		logs.Log.Errorf("Could not save milestone: %v", err)
		panic(err)
	}
	checkIsLatestMilestone(milestoneIndex, tx)

	// Trigger confirmations
	err = db.Put(db.AsKey(key, db.KEY_EVENT_CONFIRMATION_PENDING), "", nil, txn)
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
	milestoneIndex := int(convert.TritsToInt(convert.BytesToTrits(tx.ObsoleteTag[:5])).Uint64())
	trunkTransactionTrits := convert.BytesToTrits(tx.TrunkTransaction)[:243]
	normalized := transaction.NormalizedBundle(trunkTransactionTrits)[:transaction.NUMBER_OF_FRAGMENT_CHUNKS]
	digests := transaction.Digest(normalized, tx.SignatureMessageFragment, 0, 0,false)
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
