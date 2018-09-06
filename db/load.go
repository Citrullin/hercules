package db

import (
	"fmt"

	"../logs"
	"github.com/spf13/viper"
)

var Singleton Interface

func Start(config *viper.Viper) {
	database, err := Load(config)
	if err != nil {
		logs.Log.Fatal(err)
	}
	Singleton = database
}

func Load(config *viper.Viper) (Interface, error) {
	databaseType := "badger" // config.GetString("database.type")

	implementation, found := implementations[databaseType]
	if !found {
		return nil, fmt.Errorf("could not load database of type [%s]", databaseType)
	}

	database, err := implementation(config)
	if err != nil {
		return nil, fmt.Errorf("loading database [%s]: %v", databaseType, err)
	}

	return database, nil
}

var ended = false

func End() {
	ended = true
	Singleton.End()
	logs.Log.Info("DB exited")
}
