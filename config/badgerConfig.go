package config

import (
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
)

var (
	BadgerOptions         badger.Options
	BadgerCleanUpInterval time.Duration
)

func ConfigureBadger() {
	path := AppConfig.GetString("database.path")
	light := AppConfig.GetBool("light")

	BadgerCleanUpInterval = 5 * time.Minute

	BadgerOptions = badger.DefaultOptions
	BadgerOptions.Dir = path
	BadgerOptions.ValueDir = path
	BadgerOptions.ValueLogLoadingMode = options.FileIO
	BadgerOptions.TableLoadingMode = options.FileIO
	if light {
		// Source: https://github.com/dgraph-io/badger#memory-usage
		BadgerOptions.NumMemtables = 1
		BadgerOptions.NumLevelZeroTables = 1
		BadgerOptions.NumLevelZeroTablesStall = 2
		BadgerOptions.NumCompactors = 1
		BadgerOptions.MaxLevels = 5
		BadgerOptions.LevelOneSize = 256 << 18
		BadgerOptions.MaxTableSize = 64 << 18
		BadgerOptions.ValueLogFileSize = 1 << 25
		BadgerOptions.ValueLogMaxEntries = 250000
		BadgerCleanUpInterval = 2 * time.Minute
	}
}
