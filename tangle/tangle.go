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
var LastIncomingTime map[string]time.Time
var LastIncomingTimeLock = &sync.RWMutex{}
var RequestQueues map[string]*RequestQueue
var RequestQueuesLock = &sync.RWMutex{}

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
	RequestQueues = make(map[string]*RequestQueue, maxQueueSize)
	LastIncomingTime = make(map[string]time.Time)

	lowEndDevice = config.GetBool("light")

	totalTransactions = int64(db.Singleton.CountKeyCategory(db.KEY_HASH))
	totalConfirmations = int64(db.Singleton.CountKeyCategory(db.KEY_CONFIRMED))

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
		db.Singleton.RemoveKeyCategory(db.KEY_PENDING_HASH)
		db.Singleton.RemoveKeyCategory(db.KEY_PENDING_TIMESTAMP)
	}
	db.Singleton.View(func(tx db.Transaction) (e error) {
		x := 0
		return tx.ForPrefix([]byte{db.KEY_HASH}, true, func(key, value []byte) (bool, error) {
			relKey := db.AsKey(key, db.KEY_RELATION)
			relation, _ := tx.GetBytes(relKey)

			// TODO: remove pending and pending unknown?

			// Check pairs exist
			if !skipRequests &&
				(!tx.HasKey(db.AsKey(relation[:16], db.KEY_HASH)) || !tx.HasKey(db.AsKey(relation[16:], db.KEY_HASH))) {
				txBytes, _ := tx.GetBytes(db.AsKey(key, db.KEY_BYTES))
				trits := convert.BytesToTrits(txBytes)[:8019]
				t := transaction.TritsToFastTX(&trits, txBytes)
				db.Singleton.Update(func(tx db.Transaction) error {
					requestIfMissing(t.TrunkTransaction, "")
					requestIfMissing(t.BranchTransaction, "")
					return nil
				})
			}

			// Re-confirm children
			if !skipConfirmations {
				if tx.HasKey(db.AsKey(relKey, db.KEY_CONFIRMED)) {
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
