package tangle

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../transaction"
)

type Milestone struct {
	TX    *transaction.FastTX
	Index int
}

type PendingMilestone struct {
	Key         []byte
	TX2BytesKey []byte
}

const (
	COO_ADDRESS  = "KPWCHICGJZXKE9GSUDXZYUAPLHAKAHYHDXNPHENTERYMMBQOPSQIDENXKLKCEYCPVTZQLEEJVYJZV9BWU"
	COO_ADDRESS2 = "999999999999999999999999999999999999999999999999999999999999999999999999999999999"

	// TODO: for full-nodes this interval could be decreased for a little faster confirmations..?
	milestoneCheckInterval      = time.Duration(2) * time.Second
	totalMilestoneCheckInterval = time.Duration(5) * time.Minute
)

var (
	COO_ADDRESS_BYTES     = convert.TrytesToBytes(COO_ADDRESS)[:49]
	COO_ADDRESS2_BYTES    = convert.TrytesToBytes(COO_ADDRESS2)[:49]
	pendingMilestoneQueue = make(chan *PendingMilestone, maxQueueSize)
	LatestMilestone       Milestone // TODO: Access latest milestone only via GetLatestMilestone()
	LatestMilestoneLock   = &sync.RWMutex{}
	lastMilestoneCheck    = time.Now()
)

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
		has, err := requestIfMissing(hash, nil)
		if err == nil {
			if !has {
				//logs.Log.Warning("MISSING", line)
				total++
			} else {
				key := ns.HashKey(hash, ns.NamespaceMilestone)
				if !db.Singleton.HasKey(key) {
					bits, err := db.Singleton.GetBytes(ns.Key(key, ns.NamespaceBytes))
					if err != nil {
						logs.Log.Error("Couldn't get milestonetx bytes", err)
						continue
					}
					trits := convert.BytesToTrits(bits)[:8019]
					tx := transaction.TritsToTX(&trits, bits)
					trunkBytesKey := ns.HashKey(tx.TrunkTransaction, ns.NamespaceBytes)
					err = db.Singleton.PutBytes(ns.Key(key, ns.NamespaceEventMilestonePending), trunkBytesKey)
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

	loadLatestMilestoneFromDB()

	go startMilestoneChecker()
	go processPendingMilestones()
}

func loadLatestMilestoneFromDB() {
	logs.Log.Infof("Loading latest milestone from database...")
	err := db.Singleton.View(func(tx db.Transaction) error {
		var latestKey []byte
		latestIndex := 0

		LatestMilestoneLock.Lock()
		LatestMilestone = Milestone{tipFastTX, latestIndex}
		LatestMilestoneLock.Unlock()

		err := coding.ForPrefixInt(tx, ns.Prefix(ns.NamespaceMilestone), true, func(key []byte, index int) (bool, error) {
			if index >= latestIndex {
				latestKey = ns.Key(key, ns.NamespaceBytes)
				latestIndex = index
			}
			return true, nil
		})
		if err != nil {
			return err
		}

		if len(latestKey) == 0 {
			// No milestone found in database => started from snapshot
			return nil
		}

		txBytes, err := tx.GetBytes(latestKey)
		if err != nil {
			return err
		}

		trits := convert.BytesToTrits(txBytes)[:8019]
		t := transaction.TritsToTX(&trits, txBytes)

		LatestMilestoneLock.Lock()
		LatestMilestone = Milestone{t, latestIndex}
		LatestMilestoneLock.Unlock()

		return nil
	})
	if err != nil {
		logs.Log.Errorf("Loaded latest milestone from database failed! Err: %v", err)
	} else {
		logs.Log.Infof("Loaded latest milestone from database: %v", LatestMilestone.Index)
	}
}

func GetLatestMilestone() *Milestone {
	LatestMilestoneLock.RLock()
	defer LatestMilestoneLock.RUnlock()
	return &LatestMilestone
}

func setLatestMilestone(newIndex int, t *transaction.FastTX) (changed bool) {
	latestMilestone := GetLatestMilestone()
	if latestMilestone.Index < newIndex {
		// Conversion to get the Hashes
		trits := convert.BytesToTrits(t.Bytes)[:8019]
		fastTx := transaction.TritsToTX(&trits, t.Bytes)

		LatestMilestoneLock.Lock()
		defer LatestMilestoneLock.Unlock()
		LatestMilestone = Milestone{fastTx, newIndex}

		logs.Log.Infof("Latest milestone changed to: %v", newIndex)
		return true
	}
	return false
}

func addPendingMilestoneToQueue(pendingMilestone *PendingMilestone) {
	pendingMilestoneQueue <- pendingMilestone
}

/*
Runs checking of pending milestones.
*/
func startMilestoneChecker() {

	milestoneTicker := time.NewTicker(milestoneCheckInterval)
	for range milestoneTicker.C {

		// Do not check at every tick since it could take longer than a single tick
		if time.Now().Sub(lastMilestoneCheck) > totalMilestoneCheckInterval {
			// Get all pending milestones
			var pairs []PendingMilestone
			db.Singleton.View(func(tx db.Transaction) error {
				return ns.ForNamespace(tx, ns.NamespaceEventMilestonePending, true, func(key, value []byte) (bool, error) {
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
					/*
						defer func() {
							if err := recover(); err != nil {
								e = errors.New("Failed milestone check!")
							}
						}()*/
					preCheckMilestone(pair.Key, pair.TX2BytesKey, tx)
					return nil
				})
			}
			lastMilestoneCheck = time.Now()
		}
	}
}

func processPendingMilestones() {
	for pendingMilestone := range pendingMilestoneQueue {

		db.Singleton.Update(func(tx db.Transaction) (e error) {
			key := ns.Key(pendingMilestone.Key, ns.NamespaceEventMilestonePending)
			TX2BytesKey := pendingMilestone.TX2BytesKey
			if TX2BytesKey == nil {
				relation, err := tx.GetBytes(ns.Key(key, ns.NamespaceRelation))
				if err != nil {
					// The 0-index milestone TX relations doesn't exist.
					// Clearly an error here!
					tx.Remove(key)
					logs.Log.Panicf("A milestone has disappeared!")
				}
				TX2BytesKey = ns.Key(relation[:16], ns.NamespaceHash)
			}
			/*
				defer func() {
					if err := recover(); err != nil {
						e = errors.New("Failed queue milestone check!")
					}
				}()*/
			pending := preCheckMilestone(key, TX2BytesKey, tx)
			if pending {
				pendingMilestoneQueue <- pendingMilestone
			}
			return nil
		})
	}
}

func preCheckMilestone(key []byte, TX2BytesKey []byte, tx db.Transaction) (pending bool) {

	var txBytesKey = ns.Key(key, ns.NamespaceBytes)

	// 2. Check if 1-index TX already exists
	tx2Bytes, err := tx.GetBytes(TX2BytesKey)
	if err != nil {
		err := tx.PutBytes(ns.Key(TX2BytesKey, ns.NamespaceEventMilestonePairPending), key)
		if err != nil {
			logs.Log.Panicf("Could not add pending milestone pair: %v", err)
		}
		return true
	}

	// 3. Check that the 0-index TX also exists.
	txBytes, err := tx.GetBytes(txBytesKey)
	if err != nil {
		// The 0-index milestone TX doesn't exist.
		// Clearly an error here!
		// db.Remove(key, txn)
		//addPendingMilestoneToQueue(&PendingMilestone{key, TX2BytesKey})
		logs.Log.Panicf("A milestone has disappeared: %v", txBytesKey)
	}

	// Get TX objects and validate
	trits := convert.BytesToTrits(txBytes)[:8019]
	t := transaction.TritsToFastTX(&trits, txBytes)

	trits2 := convert.BytesToTrits(tx2Bytes)[:8019]
	t2 := transaction.TritsToFastTX(&trits2, tx2Bytes)

	checkMilestone(txBytesKey, t, t2, trits2, tx)
	return false
}

func checkMilestone(key []byte, t *transaction.FastTX, t2 *transaction.FastTX, trits []int, tx db.Transaction) (valid bool) {
	key = ns.Key(key, ns.NamespaceEventMilestonePending)

	discardMilestone := func() {
		err := tx.Remove(key)
		if err != nil {
			logs.Log.Panicf("Could not remove pending milestone: %v", err)
		}
		logs.Log.Panic("PANIC because of milestones!")
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
	}

	err = coding.PutInt(tx, ns.Key(key, ns.NamespaceMilestone), milestoneIndex)
	if err != nil {
		logs.Log.Panicf("Could not save milestone: %v", err)
		return false
	}
	setLatestMilestone(milestoneIndex, t)

	// Trigger confirmations
	err = addPendingConfirmation(ns.Key(key, ns.NamespaceEventConfirmationPending), int64(t.Timestamp), tx)
	if err != nil {
		logs.Log.Panicf("Could not save pending confirmation: %v", err)
		return false
	}

	return true
}

/*
Returns Milestone index if the milestone verification has been correct. Otherwise -1.
Params: tx + trits of the seconds transaction in the milestone bundle.
*/
func getMilestoneIndex(tx *transaction.FastTX, trits []int) (milestoneIndex int) {
	milestoneIndex = int(convert.TritsToInt(convert.BytesToTrits(tx.ObsoleteTag[:5])).Uint64())
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
	}
	return -1
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
	latestIndex := 0

	db.Singleton.View(func(tx db.Transaction) error {
		return coding.ForPrefixInt(tx, ns.Prefix(ns.NamespaceMilestone), true, func(key []byte, ms int) (bool, error) {
			if ms == index {
				milestoneKey = ns.Key(key, ns.NamespaceHash)
				return false, nil
			} else if acceptNearest && (ms < currentIndex) && (ms > latestIndex) {
				milestoneKey = ns.Key(key, ns.NamespaceHash)
				latestIndex = ms
			}
			return true, nil
		})
	})

	return milestoneKey
}
