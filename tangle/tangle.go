package tangle

import (
	"runtime"
	"strings"
	"sync"
	"time"

	"../config"
	"../convert"
	"../db"
	"../db/ns"
	"../logs"
	"../server"
	"../transaction"

	"github.com/coocood/freecache"
)

const (
	MWM                  = 14
	maxQueueSize         = 1000000
	reportInterval       = time.Duration(60) * time.Second
	tipRemoverInterval   = time.Duration(1) * time.Minute
	cleanupInterval      = time.Duration(10) * time.Second
	maxTipAge            = time.Duration(1) * time.Hour
	TRYTES_SIZE          = 2673
	PACKET_SIZE          = 1650
	REQ_HASH_SIZE        = 46
	HASH_SIZE            = 49 // This is not "46" on purpose, because all hashes in the DB are stored with length 49
	DATA_SIZE            = PACKET_SIZE - REQ_HASH_SIZE
	TX_TRITS_LENGTH      = 8019
	fingerPrintCacheSize = 10 * 1024 * 1024 // 10MB
)

var (
	// "constants"
	nbWorkers  = runtime.NumCPU()
	tipBytes   = convert.TrytesToBytes(strings.Repeat("9", TRYTES_SIZE))[:DATA_SIZE]
	tipTrits   = convert.BytesToTrits(tipBytes)[:TX_TRITS_LENGTH]
	tipFastTX  = transaction.TritsToTX(&tipTrits, tipBytes)
	tipHashKey = ns.HashKey(tipFastTX.Hash, ns.NamespaceHash)

	// vars
	srv                  *server.Server
	LastIncomingTime           = make(map[string]time.Time)
	LastIncomingTimeLock       = &sync.RWMutex{}
	RequestQueues              = make(map[string]*RequestQueue, maxQueueSize)
	RequestQueuesLock          = &sync.RWMutex{}
	lowEndDevice               = false
	totalTransactions    int64 = 0
	totalConfirmations   int64 = 0
	saved                      = 0
	discarded                  = 0
	outgoing                   = 0
	fingerPrintTTL             = 20 // seconds
	fingerPrintCache           = freecache.NewCache(fingerPrintCacheSize)
	cleanupTicker        *time.Ticker
	cleanupWaitGroup     = &sync.WaitGroup{}
	cleanupTickerQuit    = make(chan struct{})
	ended                = false
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

func Start() {
	srv = server.GetServer()

	// TODO: need a way to cleanup queues for disconnected/gone neighbors
	lowEndDevice = config.AppConfig.GetBool("light")

	totalTransactions = int64(ns.Count(db.Singleton, ns.NamespaceHash))
	totalConfirmations = int64(ns.Count(db.Singleton, ns.NamespaceConfirmed))

	if lowEndDevice {
		fingerPrintTTL = fingerPrintTTL * 3
	}

	// reapplyConfirmed()
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

	go outgoingRunner()

	logs.Log.Info("Tangle started!")
}

func End() {
	ended = true

	if tangleReportTicker != nil {
		tangleReportTicker.Stop()
		close(tangleReportTickerQuit)
	}
	if outgoingRunnerTicker != nil {
		outgoingRunnerTicker.Stop()
		close(outgoingRunnerTickerQuit)
	}
	if tipRemoverTicker != nil {
		tipRemoverTicker.Stop()
		close(tipRemoverTickerQuit)
	}
	if milestoneTicker != nil {
		milestoneTicker.Stop()
		close(milestoneTickerQuit)
	}
	if removeOrphanedPendingTicker != nil {
		removeOrphanedPendingTicker.Stop()
		close(removeOrphanedPendingTickerQuit)
	}
	if cleanupTicker != nil {
		cleanupTicker.Stop()
		close(cleanupTickerQuit)
	}
	close(pendingMilestoneQueueQuit)
	close(confirmQueueQuit)

	tipRemoverWaitGroup.Wait()
	milestoneTickerWaitGroup.Wait()
	removeOrphanedPendingWaitGroup.Wait()
	cleanupWaitGroup.Wait()
	outgoingRunnerWaitGroup.Wait()
	pendingMilestoneWaitGroup.Wait()
	confirmQueueWaitGroup.Wait()

	close(srv.Outgoing)
	srv.OutgoingWaitGroup.Wait()

	close(pendingMilestoneQueue)
	close(confirmQueue)
	logs.Log.Debug("Tangle module exited")
}

func cleanup() {
	cleanupWaitGroup.Add(1)
	defer cleanupWaitGroup.Done()

	interval := cleanupInterval
	if lowEndDevice {
		interval *= 3
	}

	cleanupTicker = time.NewTicker(interval)
	for {
		select {
		case <-cleanupTickerQuit:
			return

		case <-cleanupTicker.C:
			if ended {
				break
			}
			cleanupRequestQueues()
			cleanupStalledRequests()
		}
	}
}

func checkConsistency(skipRequests bool, skipConfirmations bool) {
	logs.Log.Info("Checking database consistency")
	if !skipRequests {
		ns.Remove(db.Singleton, ns.NamespacePendingHash)
		ns.Remove(db.Singleton, ns.NamespacePendingTimestamp)
	}
	db.Singleton.View(func(dbTx db.Transaction) (e error) {
		x := 0
		return ns.ForNamespace(dbTx, ns.NamespaceHash, true, func(key, value []byte) (bool, error) {
			relKey := ns.Key(key, ns.NamespaceRelation)
			relation, _ := dbTx.GetBytes(relKey)

			// TODO: remove pending and pending unknown?

			// Check pairs exist
			if !skipRequests &&
				(!dbTx.HasKey(ns.Key(relation[:16], ns.NamespaceHash)) || !dbTx.HasKey(ns.Key(relation[16:], ns.NamespaceHash))) {
				txBytes, _ := dbTx.GetBytes(ns.Key(key, ns.NamespaceBytes))
				trits := convert.BytesToTrits(txBytes)[:8019]
				tx := transaction.TritsToFastTX(&trits, txBytes)
				db.Singleton.Update(func(dbTx db.Transaction) error {
					requestIfMissing(tx.TrunkTransaction, nil)
					requestIfMissing(tx.BranchTransaction, nil)
					return nil
				})
			}

			// Re-confirm children
			if !skipConfirmations {
				if dbTx.HasKey(ns.Key(relKey, ns.NamespaceConfirmed)) {
					db.Singleton.Update(func(dbTx db.Transaction) error {
						confirmChild(relation[:16], dbTx)
						confirmChild(relation[16:], dbTx)
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
