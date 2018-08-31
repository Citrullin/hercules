package db

import (
	"fmt"

	"github.com/spf13/viper"
)

var Singleton Interface

func LoadSingleton(config *viper.Viper) error {
	database, err := Load(config)
	if err != nil {
		return err
	}
	Singleton = database
	return nil
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
