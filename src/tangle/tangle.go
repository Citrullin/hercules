package tangle

import (
	"time"
	"runtime"
	"convert"
	"strings"
	"server"
	"transaction"
	"logs"
	"math"
)

const (
	MWM             = 14
	maxQueueSize    = 100000
	reportInterval  = time.Duration(10) * time.Second
	pingInterval    = time.Duration(300) * time.Millisecond
	tipPingInterval = time.Duration(300) * time.Millisecond
	tipRemoverInterval  = time.Duration(10) * time.Second
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
var requestReplyQueues map[string]*RequestQueue

// TODO: get rid of these?
var incoming = 0
var incomingProcessed = 0
var saved = 0
var discarded = 0

func Start (s *server.Server) {
	srv = s
	requestReplyQueues = make(map[string]*RequestQueue)

	milestoneOnLoad()
	confirmOnLoad()
	tipOnLoad()

	// TODO: without snapshot: load a snapshot from file into DB. Snapshot file loader needed.

	go periodicRequest()
	go periodicTipRequest()

	for i := 0; i < int(math.Min(float64(nbWorkers), float64(4))); i++ {
		go incomingRunner()
	}
	go report()
	logs.Log.Info("Tangle started!")
}
