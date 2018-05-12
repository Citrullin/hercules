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
	"math"
)

const (
	MWM             = 14
	maxQueueSize    = 100000
	reportInterval  = time.Duration(10) * time.Second
	pingInterval    = time.Duration(50) * time.Millisecond
	tipPingInterval = time.Duration(100) * time.Millisecond
	tipRemoverInterval  = time.Duration(10) * time.Second
	maxTipAge           = time.Duration(1) * time.Hour
	reRequestInterval   = time.Duration(3) * time.Second
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

type RequestQueue chan *Request
type MessageQueue chan *Message
type TXQueue chan *transaction.FastTX

// "constants"
var nbWorkers = runtime.NumCPU()
var tipBytes = convert.TrytesToBytes(strings.Repeat("9", 2673))[:1604]
var tipTrits = convert.BytesToTrits(tipBytes)[:8019]
var tipFastTX = transaction.TritsToFastTX(&tipTrits, tipBytes)
var fingerprintTTL = time.Duration(1) * time.Minute

var srv *server.Server
var requestReplyQueue RequestQueue
var requestReplyQueues map[string]*RequestQueue
var outgoingQueue MessageQueue
var incomingQueue MessageQueue
var pendings map[string]int64
var PendingsLocker = &sync.Mutex{}

// TODO: get rid of these?
var incoming = 0
var incomingProcessed = 0
var saved = 0
var discarded = 0

func Start (s *server.Server) {
	srv = s
	incomingQueue = make(MessageQueue, maxQueueSize)
	outgoingQueue = make(MessageQueue, maxQueueSize)
	requestReplyQueue = make(RequestQueue, maxQueueSize)
	requestReplyQueues = make(map[string]*RequestQueue)
	pendings = make(map[string]int64)

	loadPendings()
	milestoneOnLoad()
	confirmOnLoad()
	tipOnLoad()

	// TODO: without snapshot: load a snapshot from file into DB. Snapshot file loader needed.

	go periodicRequest()
	go periodicTipRequest()

	for i := 0; i < int(math.Min(float64(nbWorkers), float64(4))); i++ {
		go incomingRunner()
		go responseRunner()
		//go listenToIncoming()
		//go requestReplyRunner()
	}
	go report()
	logs.Log.Info("Tangle started!")
}
