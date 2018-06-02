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
	"os"
	"bufio"
	"io"
	"github.com/dgraph-io/badger"
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

	tipOnLoad()
	pendingOnLoad()
	milestoneOnLoad()
	confirmOnLoad()

	//loadMissing()

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

// TODO: remove this? Or add an API interface?
func loadMissing() error {
	logs.Log.Info("Loading missing...")
	total := 0
	f, err := os.OpenFile("TXs", os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			logs.Log.Fatalf("read file line error: %v", err)
			return err
		}
		hash := convert.TrytesToBytes(line)[:49]
		//key := db.GetByteKey(hash, db.KEY_HASH)
		has, err := requestIfMissing(hash, "", nil)
		if err == nil {
			if !has {
				logs.Log.Warning("MISSING", line)
				total++
			} else {
				/*confirmed := db.Has(db.AsKey(key, db.KEY_CONFIRMED), nil)
				txBytes, err := db.GetBytes(db.AsKey(key, db.KEY_BYTES), nil)
				if err != nil {
					logs.Log.Error("!!! BYTES NOT FOUND", line)
					continue
				}
				trits := convert.BytesToTrits(txBytes)[:8019]
				tx := transaction.TritsToFastTX(&trits, txBytes)
				logs.Log.Warning("HAS", line, confirmed, tx.Value, convert.BytesToTrytes(tx.Bundle)[:81])
				dwellDeeper(line) */
			}
		} else {
			logs.Log.Error("ERR", err)
		}
	}
	logs.Log.Info("Loaded missing", total)

	return nil
}

func dwellDeeper(line string) {
	hash := convert.TrytesToBytes(line)[:49]
	_ = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := db.GetByteKey(hash, db.KEY_APPROVEE)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			approveeKey := it.Item().Key()[16:]
			has := db.Has(db.AsKey(approveeKey, db.KEY_CONFIRMED), txn)
			logs.Log.Debug("   --->", approveeKey, has)
		}
		return nil
	})
}
