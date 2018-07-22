package tangle

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"../convert"
	"../db"
	"../logs"
	"../server"
	"../transaction"
	"github.com/spf13/viper"
	"github.com/dgraph-io/badger"
)

const (
	MWM                = 14
	maxQueueSize       = 1000000
	reportInterval     = time.Duration(60) * time.Second
	tipRemoverInterval = time.Duration(1) * time.Minute
	cleanupInterval    = time.Duration(10) * time.Second
	maxTipAge          = time.Duration(1) * time.Hour
)

type Message struct {
	Bytes             *[]byte
	Requested         *[]byte
	IPAddressWithPort string
}

type Request struct {
	Requested []byte
	Tip       bool
}

type IncomingTX struct {
	TX                *transaction.FastTX
	IPAddressWithPort string
	Bytes             *[]byte
}

type RequestQueue chan *Request

// "constants"
var nbWorkers = runtime.NumCPU()
var tipBytes = convert.TrytesToBytes(strings.Repeat("9", 2673))[:1604]
var tipTrits = convert.BytesToTrits(tipBytes)[:8019]
var tipFastTX = transaction.TritsToTX(&tipTrits, tipBytes)
var tipHashKey = db.GetByteKey(tipFastTX.Hash, db.KEY_HASH)

var srv *server.Server
var config *viper.Viper
var requestQueues map[string]*RequestQueue
var requestLocker = &sync.RWMutex{}
var pendingRequestLocker = &sync.RWMutex{}

var lowEndDevice = false
var totalTransactions int64 = 0
var totalConfirmations int64 = 0
var incoming = 0
var incomingProcessed = 0
var saved = 0
var discarded = 0
var outgoing = 0

func Start(s *server.Server, cfg *viper.Viper) {
	config = cfg
	srv = s
	// TODO: need a way to cleanup queues for disconnected/gone neighbors
	requestQueues = make(map[string]*RequestQueue, maxQueueSize)

	lowEndDevice = config.GetBool("light")

	totalTransactions = int64(db.Count(db.KEY_HASH))
	totalConfirmations = int64(db.Count(db.KEY_CONFIRMED))

	// reapplyConfirmed()
	fingerprintsOnLoad()
	tipOnLoad()
	pendingOnLoad()
	milestoneOnLoad()
	confirmOnLoad()
	// checkConsistency(false, false)


	// This had to be done due to the tangle split in May 2018.
	// Might need this in the future for whatever reason?
	// LoadMissingMilestonesFromFile("milestones.txt")

	for i := 0; i < nbWorkers; i++ {
		go incomingRunner()
	}

	go report()
	go cleanup()
	logs.Log.Info("Tangle started!")

	go func() {
		interval := time.Duration(2) * time.Millisecond
		if lowEndDevice {
			interval = interval * 5
		}
		for {
			outgoingRunner()
			time.Sleep(time.Duration(2) * time.Millisecond)
		}
	}()
}

func cleanup () {
	flushTicker := time.NewTicker(cleanupInterval)
	for range flushTicker.C {
		cleanupFingerprints()
		cleanupRequestQueues()
	}
}

func checkConsistency (skipRequests bool, skipConfirmations bool) {
	logs.Log.Info("Checking database consistency")
	if !skipRequests {
		db.RemoveAll(db.KEY_PENDING_HASH)
		db.RemoveAll(db.KEY_PENDING_TIMESTAMP)
	}
	db.DB.View(func(txn *badger.Txn) (e error) {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_HASH}
		x := 0
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			relKey := db.AsKey(key, db.KEY_RELATION)
			relation, _ := db.GetBytes(relKey, txn)

			// TODO: remove pending and pending unknown?

			// Check pairs exist
			if !skipRequests && (
				!db.Has(db.AsKey(relation[:16], db.KEY_HASH), txn) ||
					!db.Has(db.AsKey(relation[16:], db.KEY_HASH), txn)) {
				txBytes, _ := db.GetBytes(db.AsKey(key, db.KEY_BYTES), txn)
				trits := convert.BytesToTrits(txBytes)[:8019]
				tx := transaction.TritsToFastTX(&trits, txBytes)
				db.DB.Update(func(txn *badger.Txn) error {
					requestIfMissing(tx.TrunkTransaction, "")
					requestIfMissing(tx.BranchTransaction, "")
					return nil
				})
			}

			// Re-confirm children
			if !skipConfirmations {
				if db.Has(db.AsKey(relKey, db.KEY_CONFIRMED), txn) {
					db.DB.Update(func(txn *badger.Txn) error {
						confirmChild(relation[:16], txn)
						confirmChild(relation[16:], txn)
						return nil
					})
				}
			}
			x++

			if x % 10000 == 0 {
				logs.Log.Debug("Processed", x)
			}
		}
		return nil
	})
}