package db_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestBadger(t *testing.T) {
	InterfaceSuite(t, func() (*viper.Viper, func()) {
		path, err := ioutil.TempDir("", "badger")
		require.NoError(t, err)

		config := viper.New()
		config.Set("database.type", "badger")
		config.Set("database.path", path)

		return config, func() {
			require.NoError(t, os.RemoveAll(path))
		}
	})
}
