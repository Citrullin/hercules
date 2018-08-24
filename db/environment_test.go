package db_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"."
)

type environment struct {
	db       *db.Badger
	tearDown func()
}

func setUpTestEnvironment(tb testing.TB) *environment {
	path, err := ioutil.TempDir("", "badger")
	require.NoError(tb, err)

	db, err := db.NewBadger(path, false)
	require.NoError(tb, err)

	return &environment{
		db: db,
		tearDown: func() {
			require.NoError(tb, os.RemoveAll(path))
		},
	}
}
