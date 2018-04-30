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
	"api"
)

var apiPort string

func init() {
	var ns string
	var port string
	flag.StringVar(&port, "u", "14600", "UDP Port")
	flag.StringVar(&apiPort, "p", "14265", "Node Port to listen to")
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
	api.Start(":" + apiPort)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		for range ch {
			// Clean exit
			log.Println("Hercules is shutting down. Please wait...")
			go func () {
				time.Sleep(time.Duration(5000) * time.Millisecond)
				os.Exit(0)
			}()
			go server.End()
			db.End()
		}
	}()
	for {
		time.Sleep(time.Second)
	}
}
