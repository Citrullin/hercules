package db_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"."
)

type environment struct {
	db       db.Interface
	tearDown func()
}

func setUpTestEnvironment(tb testing.TB) *environment {
	path, err := ioutil.TempDir("", "badger")
	require.NoError(tb, err)

	config := viper.New()
	config.Set("database.type", "badger")
	config.Set("database.path", path)

	db, err := db.Load(config)
	require.NoError(tb, err)

	return &environment{
		db: db,
		tearDown: func() {
			require.NoError(tb, os.RemoveAll(path))
		},
	}
}
