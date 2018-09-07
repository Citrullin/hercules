package tangle

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../server"
	"../transaction"
	"github.com/spf13/viper"
)

const (
	MWM                = 14
	maxQueueSize       = 1000000
	reportInterval     = time.Duration(60) * time.Second
	tipRemoverInterval = time.Duration(1) * time.Minute
	cleanupInterval    = time.Duration(10) * time.Second
	maxTipAge          = time.Duration(1) * time.Hour
	TRYTES_SIZE        = 2673
	PACKET_SIZE        = 1650
	REQ_HASH_SIZE      = 46
	HASH_SIZE          = 49 // This is not "46" on purpose, because all hashes in the DB are stored with length 49
	DATA_SIZE          = PACKET_SIZE - REQ_HASH_SIZE
	TX_TRITS_LENGTH    = 8019
)

type Message struct {
	Bytes     *[]byte
	Requested *[]byte
	Neighbor  *server.Neighbor
}

type Request struct {
	Requested []byte
	Tip       bool
}

type IncomingTX struct {
	TX       *transaction.FastTX
	Neighbor *server.Neighbor
	Bytes    *[]byte
}

type RequestQueue chan *Request

var (
	// "constants"
	nbWorkers  = runtime.NumCPU()
	tipBytes   = convert.TrytesToBytes(strings.Repeat("9", TRYTES_SIZE))[:DATA_SIZE]
	tipTrits   = convert.BytesToTrits(tipBytes)[:TX_TRITS_LENGTH]
	tipFastTX  = transaction.TritsToTX(&tipTrits, tipBytes)
	tipHashKey = ns.HashKey(tipFastTX.Hash, ns.NamespaceHash)

	// vars
	srv                  *server.Server
	config               *viper.Viper
	LastIncomingTime     map[string]time.Time
	LastIncomingTimeLock = &sync.RWMutex{}
	RequestQueues        map[string]*RequestQueue
	RequestQueuesLock          = &sync.RWMutex{}
	lowEndDevice               = false
	totalTransactions    int64 = 0
	totalConfirmations   int64 = 0
	incoming                   = 0
	incomingProcessed          = 0
	saved                      = 0
	discarded                  = 0
	outgoing                   = 0
)

func Start(cfg *viper.Viper) {
	config = cfg
	srv = server.GetServer()

	// TODO: need a way to cleanup queues for disconnected/gone neighbors
	RequestQueues = make(map[string]*RequestQueue, maxQueueSize)
	LastIncomingTime = make(map[string]time.Time)

	lowEndDevice = config.GetBool("light")

	totalTransactions = int64(ns.Count(db.Singleton, ns.NamespaceHash))
	totalConfirmations = int64(ns.Count(db.Singleton, ns.NamespaceConfirmed))

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
			time.Sleep(interval)
		}
	}()
}

func cleanup() {
	interval := cleanupInterval
	if lowEndDevice {
		interval *= 3
	}
	cleanupTicker := time.NewTicker(interval)
	for range cleanupTicker.C {
		cleanupFingerprints()
		cleanupRequestQueues()
		cleanupStalledRequests()
	}
}

func checkConsistency(skipRequests bool, skipConfirmations bool) {
	logs.Log.Info("Checking database consistency")
	if !skipRequests {
		ns.Remove(db.Singleton, ns.NamespacePendingHash)
		ns.Remove(db.Singleton, ns.NamespacePendingTimestamp)
	}
	db.Singleton.View(func(tx db.Transaction) (e error) {
		x := 0
		return ns.ForNamespace(tx, ns.NamespaceHash, true, func(key, value []byte) (bool, error) {
			relKey := ns.Key(key, ns.NamespaceRelation)
			relation, _ := coding.GetBytes(tx, relKey)

			// TODO: remove pending and pending unknown?

			// Check pairs exist
			if !skipRequests &&
				(!tx.HasKey(ns.Key(relation[:16], ns.NamespaceHash)) || !tx.HasKey(ns.Key(relation[16:], ns.NamespaceHash))) {
				txBytes, _ := coding.GetBytes(tx, ns.Key(key, ns.NamespaceBytes))
				trits := convert.BytesToTrits(txBytes)[:8019]
				t := transaction.TritsToFastTX(&trits, txBytes)
				db.Singleton.Update(func(tx db.Transaction) error {
					requestIfMissing(t.TrunkTransaction, nil)
					requestIfMissing(t.BranchTransaction, nil)
					return nil
				})
			}

			// Re-confirm children
			if !skipConfirmations {
				if tx.HasKey(ns.Key(relKey, ns.NamespaceConfirmed)) {
					db.Singleton.Update(func(tx db.Transaction) error {
						confirmChild(relation[:16], tx)
						confirmChild(relation[16:], tx)
						return nil
					})
				}
			}
			x++

			if x%10000 == 0 {
				logs.Log.Debug("Processed", x)
			}

			return true, nil
		})
	})
}
