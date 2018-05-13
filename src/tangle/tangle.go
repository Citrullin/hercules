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
)

const (
	MWM             = 14
	maxQueueSize    = 20000
	reportInterval  = time.Duration(10) * time.Second
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
var requestQueues map[string]*RequestQueue
var replyQueues map[string]*RequestQueue

var pendingHashes [][]byte
var pendingLocker = &sync.Mutex{}

// TODO: get rid of these?
var incoming = 0
var incomingProcessed = 0
var saved = 0
var discarded = 0

func Start (s *server.Server) {
	srv = s
	requestQueues = make(map[string]*RequestQueue)
	replyQueues = make(map[string]*RequestQueue)

	milestoneOnLoad()
	confirmOnLoad()
	tipOnLoad()

	// TODO: without snapshot: load a snapshot from file into DB. Snapshot file loader needed.

	for i := 0; i < nbWorkers; i++ {
		go runner()
	}

	go report()
	logs.Log.Info("Tangle started!")
}

func runner () {
	for {
		select {
		case msg := <- srv.Incoming:
			incomingRunner(msg)
		default:
			outgoingRunner()
		}
		time.Sleep(1)
	}
}
