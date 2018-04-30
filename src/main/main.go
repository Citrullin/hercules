package main

import (
	"server"
	"flag"
	"strings"
	"time"
	"tangle"
	"db"
	"os"
	"path"
	"os/signal"
	"log"
	"runtime"
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
	runtime.GOMAXPROCS(runtime.NumCPU())
	StartHercules()
}

var serverConfig *server.ServerConfig

func StartHercules () {
	cwd, _ := os.Getwd()
	db.Load(&db.DatabaseConfig{path.Join(cwd, "data"), 10})
	srv := server.Create(serverConfig)
	tangle.Start(srv)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		for range ch {
			// Clean exit
			log.Println("Hercules is shutting down. Please wait...")
			server.End()
			db.End()
			os.Exit(0)
		}
	}()
	for {
		time.Sleep(time.Second)
	}
}
