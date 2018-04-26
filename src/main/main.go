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
	"crypt"
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
	MWM = 14
)

var serverConfig *server.ServerConfig
var srv *server.Server
var tng *tangle.Tangle
var seenQueue map[[md5.Size]byte]bool

func StartHercules () {
	seenQueue = make(map[[md5.Size]byte]bool)
	cwd, _ := os.Getwd()
	db.LoadDB(&db.DatabaseConfig{path.Join(cwd, "data"), 10})
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
			time.Sleep(time.Duration(1500) * time.Millisecond)
			db.EndDB()
			os.Exit(0)
		}
	}()

	go listenForTangleRequests()
	listenForIncomingMessage()
}

func MakeAnyTXBytes () []byte {
	trytes := strings.Repeat("9", 2673)
	req := strings.Repeat("9", 81)
	trits := convert.TrytesToBytes(trytes)
	riq := convert.TrytesToBytes(req)
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
		// trytes := bytesToTrytes(inc.Msg[:1604])[:2673]
		// request := bytesToTrytes(inc.Msg[1604:]) + strings.Repeat("9", 4)
		bytes := inc.Msg[:1604]
		req := inc.Msg[1604:]
		if !crypt.IsValidPoW(convert.BytesToTrits(bytes), MWM) {
			continue
		}
		tng.Incoming <- &tangle.Message{&bytes,&req, inc.Addr}
	}
}

func listenForTangleRequests() {
	for out := range tng.Outgoing {
		//fmt.Println("L", len(*out.Trytes), len(*out.Requested))
		data := append((*out.Trytes)[:1604], (*out.Requested)[:46]...)
		srv.Outgoing <- &server.Message{out.Addr, data}
	}
}
