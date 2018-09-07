package db

import (
	"fmt"

	"../logs"
)

var Singleton Interface

func Start() {
	database, err := loadDB()
	if err != nil {
		logs.Log.Fatal(err)
	}
	Singleton = database
}

func loadDB() (Interface, error) {
	databaseType := "badger" // config.GetString("database.type")

	implementation, found := implementations[databaseType]
	if !found {
		return nil, fmt.Errorf("could not load database of type [%s]", databaseType)
	}

	database, err := implementation()
	if err != nil {
		return nil, fmt.Errorf("loading database [%s]: %v", databaseType, err)
	}

	return database, nil
}

func End() {
	Singleton.End()
	logs.Log.Debug("DB exited")
}
