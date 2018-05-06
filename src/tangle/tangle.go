package tangle

import (
	"time"
	"runtime"
	"convert"
	"strings"
	"server"
	"transaction"
	"log"
)

const (
	MWM             = 14
	maxQueueSize    = 100000
	reportInterval  = time.Duration(10) * time.Second
	pingInterval    = time.Duration(10) * time.Millisecond
	tipPingInterval = time.Duration(100) * time.Millisecond
	)

type Message struct {
	Bytes     *[]byte
	Requested *[]byte
	Addr      string
}

type Request struct {
	Requested []byte
	Addr string
	Tip bool
}

type RequestQueue chan *Request
type MessageQueue chan *Message
type TXQueue chan *transaction.FastTX

// "constants"
var nbWorkers = runtime.NumCPU()
var latestMilestoneKey = []byte("MilestoneLatest")
var tipBytes = convert.TrytesToBytes(strings.Repeat("9", 2673))[:1604]
var tipTrits = convert.BytesToTrits(tipBytes)[:8019]
var tipFastTX = transaction.TritsToFastTX(&tipTrits, tipBytes)
var fingerprintTTL = time.Duration(10) * time.Minute
var reRequestTTL = time.Duration(5) * time.Second

var srv *server.Server
var requestReplyQueue RequestQueue
var outgoingQueue MessageQueue
var incomingQueue MessageQueue

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

	milestoneOnLoad()
	confirmOnLoad()
	tipOnLoad()

	// TODO: without snapshot: load a snapshot from file into DB. Snapshot file loader needed.

	go periodicRequest()
	go periodicTipRequest()

	for i := 0; i < 3; i++ {
		go incomingRunner()
		go responseRunner()
		go listenToIncoming()
		go requestReplyRunner()
	}
	go report()
	log.Println("Tangle started!")
}
