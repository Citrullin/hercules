package db_test

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"."
)

type environment struct {
	db       db.Interface
	tearDown func()
}

type setUpFunc func() (*viper.Viper, func())

func setUpTestEnvironment(tb testing.TB, setUpFn setUpFunc) *environment {
	config, tearDownFn := setUpFn()

	db, err := db.Load(config)
	require.NoError(tb, err)

	return &environment{
		db: db,
		tearDown: func() {
			tearDownFn()
		},
	}
}
