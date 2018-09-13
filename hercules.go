package main

import (
	"net/http"
	"os"
	"os/signal"
	"runtime"
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

	_ "net/http/pprof"

	"github.com/pkg/profile"
)

var (
	herculesDeathWaitGroup = &sync.WaitGroup{}
)

func main() {
	herculesDeathWaitGroup.Add(1)

	startBasicFunctionality()
	logs.Log.Info("Starting Hercules. Please wait...")

	if config.AppConfig.GetBool("debug") {
		defer profile.Start().Stop()

		runtime.SetMutexProfileFraction(5)
		go http.ListenAndServe(":6060", nil) // pprof Server for Debbuging Mutexes
	}

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
	herculesDeathWaitGroup.Done()
}
