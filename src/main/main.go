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
	"crypt"
	"log"
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
	flushInterval = time.Duration(10) * time.Second
	seenQueueSize = 10000
	MWM = 14
)

var serverConfig *server.ServerConfig
var srv *server.Server
var tng *tangle.Tangle

func StartHercules () {
	cwd, _ := os.Getwd()
	db.Load(&db.DatabaseConfig{path.Join(cwd, "data"), 10})
	tng = tangle.Start()
	srv = server.Create(serverConfig)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		for range ch {
			// Clean exit
			log.Println("Hercules is shutting down. Please wait...")
			server.End()
			time.Sleep(time.Duration(5000) * time.Millisecond)
			db.End()
			os.Exit(0)
		}
	}()

	go listenForTangleRequests()
	listenForIncomingMessage()
}

func listenForIncomingMessage () {
	for inc := range srv.Incoming {
		bytes := inc.Msg[:1604]
		req := inc.Msg[1604:1650]
		if !crypt.IsValidPoW(convert.BytesToTrits(bytes), MWM) {
			continue
		}

		tng.Incoming <- &tangle.Message{&bytes,&req, inc.Addr}
	}
}

func listenForTangleRequests() {
	for out := range tng.Outgoing {
		data := append((*out.Bytes)[:1604], (*out.Requested)[:46]...)
		srv.Outgoing <- &server.Message{out.Addr, data}
	}
}
