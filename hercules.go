package main

import (
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
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
		defer profile.Start(profile.NoShutdownHook).Stop()

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
	debug.SetGCPercent(20) // for the use of freecache, could also be improved if not optimal

	utils.Hello()
	time.Sleep(500 * time.Millisecond)
}

func gracefullyDies() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-ch // waits for death signal

	logs.Log.Infof("Caught signal '%s': Hercules is shutting down. Please wait...", sig)
	api.End()
	server.End()
	db.End()

	logs.Log.Info("Bye!")
	herculesDeathWaitGroup.Done()
}
