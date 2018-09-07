package main

import (
	"os"
	"os/signal"
	"sync"
	"time"

	"./api"
	"./config"
	"./db"
	"./logs"
	"./server"
	"./snapshot"
	"./tangle"
	"./utils"

	"github.com/pkg/profile"
)

var herculesDeathWaitGroup = &sync.WaitGroup{}

func main() {
	herculesDeathWaitGroup.Add(1)

	defer profile.Start().Stop()

	startBasicFunctionality()
	logs.Log.Info("Starting Hercules. Please wait...")

	db.Start()
	snapshot.Start()

	server.Start()
	tangle.Start()
	api.Start()

	go gracefullyDies()
	herculesDeathWaitGroup.Wait()
}

func startBasicFunctionality() {
	logs.Start()
	config.Start()

	utils.Hello()
	time.Sleep(500 * time.Millisecond)
}

func gracefullyDies() {
	ch := make(chan os.Signal, 10)
	signal.Notify(ch, os.Interrupt)
	signal.Notify(ch, os.Kill)

	<-ch // waits for death signal

	logs.Log.Info("Hercules is shutting down. Please wait...")
	api.End()
	server.End()
	db.End()

	logs.Log.Info("Bye!")
	herculesDeathWaitGroup.Add(-1)
}
