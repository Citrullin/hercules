package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testKey   = []byte("key")
	testValue = []byte("value")
)

func InterfaceSuite(t *testing.T, setUpFn setUpFunc) {
	t.Run("PutBytes", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue))
	})

	t.Run("GetBytes", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue))

		value, err := e.db.GetBytes(testKey)
		require.NoError(t, err)

		assert.Equal(t, "value", string(value))
	})

	t.Run("HasKey", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue))

		assert.True(t, e.db.HasKey(testKey))
	})

	t.Run("Remove", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue))

		require.NoError(t,
			e.db.Remove(testKey))

		assert.False(t, e.db.HasKey(testKey))
	})

	t.Run("RemovePrefix", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t, e.db.PutBytes([]byte("abc"), testValue))
		require.NoError(t, e.db.PutBytes([]byte("aac"), testValue))

		require.NoError(t,
			e.db.RemovePrefix([]byte("aa")))

		assert.True(t, e.db.HasKey([]byte("abc")))
		assert.False(t, e.db.HasKey([]byte("aac")))
	})

	t.Run("CountPrefix", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t, e.db.PutBytes([]byte("abc"), testValue))
		require.NoError(t, e.db.PutBytes([]byte("aac"), testValue))

		assert.Equal(t, 2, e.db.CountPrefix([]byte("a")))
	})
}
