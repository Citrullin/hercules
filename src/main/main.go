package main

import (
	"server"
	"flag"
	"strings"
	"time"
	"tangle"
	"convert"
	"db"
	"os"
	"path"
	"os/signal"
	"crypto/md5"
)

func init() {
	var ns string
	var port string
	flag.StringVar(&port, "p", "14600", "Node Port to listen to")
	flag.StringVar(&ns, "n", "", "Initial Node neighbors")

	flag.Parse()

	neighbors := strings.Split(ns, " ")
	for i := range neighbors {
		neighbors[i] = strings.TrimSpace(neighbors[i])
	}
	serverConfig = &server.ServerConfig{
		Neighbors: neighbors,
		Port: port}
}

func main () {
	StartHercules()
}

const (
	flushInterval = time.Duration(3) * time.Second
	seenQueueSize = 10000
)

var serverConfig *server.ServerConfig
var srv *server.Server
var tng *tangle.Tangle
var seenQueue map[[md5.Size]byte]bool

func StartHercules () {
	seenQueue = make(map[[md5.Size]byte]bool)
	cwd, _ := os.Getwd()
	db.Load(&db.DatabaseConfig{path.Join(cwd, "data"), 10})
	tng = tangle.Start()
	srv = server.Create(serverConfig)
	flushTicker := time.NewTicker(flushInterval)
	go func() {
		for range flushTicker.C {
			srv.Outgoing <- &server.Message{Msg: MakeAnyTXBytes()}
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		for range ch {
			// Clean exit
			server.End()
			db.End()
			time.Sleep(time.Duration(500) * time.Millisecond)
			os.Exit(0)
		}
	}()

	go listenForTangleRequests()
	listenForIncomingMessage()
}

func MakeAnyTXBytes () []byte {
	trytes := strings.Repeat("9", 2673)
	req := strings.Repeat("9", 81)
	trits := trytesToBytes(trytes)
	riq := trytesToBytes(req)
	return append(trits, riq[:46]...)
}

func listenForIncomingMessage () {
	for inc := range srv.Incoming {
		if len(seenQueue) == seenQueueSize {
			seenQueue = make(map[[md5.Size]byte]bool)
		}
		hash := md5.Sum(inc.Msg)
		_, ok := seenQueue[hash]
		if ok { continue }
		seenQueue[hash] = true
		trytes := bytesToTrytes(inc.Msg[:1604])[:2673]
		request := bytesToTrytes(inc.Msg[1604:]) + strings.Repeat("9", 4)
		tng.Incoming <- &tangle.Message{trytes,request, inc.Addr}
	}
}

func listenForTangleRequests() {
	for out := range tng.Outgoing {
		trits := trytesToBytes(out.Trytes)[:1604]
		riq := trytesToBytes(out.Requested)
		data := append(trits, riq[:46]...)
		srv.Outgoing <- &server.Message{out.Addr, data}
	}
}

func trytesToBytes (trytes string) []byte {
	return convert.TritsToBytes(convert.TrytesToTrits(trytes))
}
func bytesToTrytes (bytes []byte) string {
	return convert.TritsToTrytes(convert.BytesToTrits(bytes))
}
