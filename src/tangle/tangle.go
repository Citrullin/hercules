package tangle

import (
	"time"
	"runtime"
	"convert"
	"strings"
	"server"
	"transaction"
	"logs"
	"sync"
	"db"
	"github.com/spf13/viper"
)

const (
	MWM             = 14
	maxQueueSize    = 1000000
	reportInterval  = time.Duration(60) * time.Second
	tipRemoverInterval  = time.Duration(1) * time.Minute
	maxTipAge           = time.Duration(1) * time.Hour
	)

type Message struct {
	Bytes     *[]byte
	Requested *[]byte
	Addr      string
}

type Request struct {
	Requested []byte
	Tip bool
}

type IncomingTX struct {
	TX    *transaction.FastTX
	Addr  string
	Bytes *[]byte
}

type RequestQueue chan *Request
type MessageQueue chan *Message
type TXQueue chan *IncomingTX

// "constants"
var nbWorkers = runtime.NumCPU()
var tipBytes = convert.TrytesToBytes(strings.Repeat("9", 2673))[:1604]
var tipTrits = convert.BytesToTrits(tipBytes)[:8019]
var tipFastTX = transaction.TritsToTX(&tipTrits, tipBytes)
var tipHashKey = db.GetByteKey(tipFastTX.Hash, db.KEY_HASH)

var srv *server.Server
var config *viper.Viper
var requestQueues map[string]*RequestQueue
var replyQueues map[string]*RequestQueue
var requestLocker = &sync.RWMutex{}
var pendingRequestLocker = &sync.RWMutex{}
var replyLocker = &sync.RWMutex{}

var txQueue TXQueue

// TODO: get rid of these?
var incoming = 0
var incomingProcessed = 0
var saved = 0
var discarded = 0
var outgoing = 0

func Start (s *server.Server, cfg *viper.Viper) {
	config = cfg
	srv = s
	requestQueues = make(map[string]*RequestQueue)
	replyQueues = make(map[string]*RequestQueue)
	txQueue = make(TXQueue, maxQueueSize)

	//loadIRISnapshot("snapshotMainnet.txt","previousEpochsSpentAddresses.txt", 1525017600)
	//err := snapshot.SaveSnapshot(config.GetString("snapshots.path"))
	//snapshot.OnSnapshotsLoad()
	//err := snapshot.LoadSnapshot("snapshots/1525017600.snap")
	//logs.Log.Fatal("saveSnapshot result:", err)
	/*
	for {
		time.Sleep(time.Second)
	}
	return
	*/

	tipOnLoad()
	pendingOnLoad()
	milestoneOnLoad()
	confirmOnLoad()

	// TODO: without snapshot: load a snapshot from file into DB. Snapshot file loader needed.

	for i := 0; i < nbWorkers; i++ {
		go incomingRunner()
	}

	go report()
	logs.Log.Info("Tangle started!")
	server.Start()

	go runner()
}

func runner () {
	for {
		select {
		case incomingTX := <- txQueue:
			processIncomingTX(incomingTX)
		default:
		}
		outgoingRunner()
		time.Sleep(time.Duration(len(srv.Incoming) * 10000))
	}
}
