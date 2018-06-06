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

var lowEndDevice = false
var totalTransactions int64 = 0
var totalConfirmations int64 = 0
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

	lowEndDevice = config.GetBool("light")

	totalTransactions = int64(db.Count(db.KEY_HASH))
	totalConfirmations = int64(db.Count(db.KEY_CONFIRMED))

	tipOnLoad()
	pendingOnLoad()
	milestoneOnLoad()
	confirmOnLoad()

	// This had to be done due to the tangle split in May 2018.
	// Might need this in the future for whatever reason?
	// LoadMissingMilestonesFromFile("milestones.txt")

	for i := 0; i < nbWorkers; i++ {
		go incomingRunner()
	}

	go report()
	go runner()
	logs.Log.Info("Tangle started!")
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
