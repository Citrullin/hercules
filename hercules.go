package main

import (
	"encoding/json"
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
	"github.com/spf13/viper"
)

var viperConfig *viper.Viper

func init() {
	logs.Setup()
	viperConfig := config.LoadConfig()
	logs.SetConfig(viperConfig)

	cfg, _ := json.MarshalIndent(viperConfig.AllSettings(), "", "  ")
	logs.Log.Debugf("Following settings loaded: \n %+v", string(cfg))
}

var herculesDeathWaitGroup = &sync.WaitGroup{}

func main() {
	herculesDeathWaitGroup.Add(1)

	defer profile.Start().Stop()
	utils.Hello(viperConfig)
	time.Sleep(time.Duration(500) * time.Millisecond)

	logs.Log.Info("Starting Hercules. Please wait...")

	db.Start(viperConfig)
	snapshot.Start(viperConfig)

	server.Start(viperConfig)
	tangle.Start(viperConfig)
	api.Start(viperConfig)

	go gracefullyDies()
	herculesDeathWaitGroup.Wait()
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
