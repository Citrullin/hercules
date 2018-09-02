package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
	"./coding"
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
			e.db.PutBytes(testKey, testValue, nil))
	})

	t.Run("GetBytes", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue, nil))

		value, err := e.db.GetBytes(testKey)
		require.NoError(t, err)

		assert.Equal(t, "value", string(value))
	})

	t.Run("HasKey", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue, nil))

		assert.True(t, e.db.HasKey(testKey))
	})

	t.Run("Get", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, []byte{0x4, 0x4, 0x0, 0xff, 0xf6}, nil))

		var value int
		err := e.db.Get(testKey, &value)
		require.NoError(t, err)
		assert.Equal(t, 123, value)
	})

	t.Run("Remove", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue, nil))

		require.NoError(t,
			e.db.Remove(testKey))

		assert.False(t, e.db.HasKey(testKey))
	})

	t.Run("RemovePrefix", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t, e.db.PutBytes([]byte("abc"), testValue, nil))
		require.NoError(t, e.db.PutBytes([]byte("aac"), testValue, nil))

		require.NoError(t,
			e.db.RemovePrefix([]byte("aa")))

		assert.True(t, e.db.HasKey([]byte("abc")))
		assert.False(t, e.db.HasKey([]byte("aac")))
	})

	t.Run("RemoveKeysFromCategoryBefore", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		key := db.AsKey(testKey, db.KEY_EDGE)
		require.NoError(t, coding.PutInt64(e.db, key, 1000))

		assert.Equal(t, 1, e.db.RemoveKeysFromCategoryBefore(db.KEY_EDGE, 2000))

		assert.False(t, e.db.HasKey(key))
	})

	t.Run("CountPrefix", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t, e.db.PutBytes([]byte("abc"), testValue, nil))
		require.NoError(t, e.db.PutBytes([]byte("aac"), testValue, nil))

		assert.Equal(t, 2, e.db.CountPrefix([]byte("a")))
	})

	t.Run("IncrementBy", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t, coding.PutInt64(e.db, testKey, 1000))

		value, err := e.db.IncrementBy(testKey, 10, true)
		require.NoError(t, err)

		assert.Equal(t, int64(1010), value)

		value, err = e.db.GetInt64(testKey)
		require.NoError(t, err)
		assert.Equal(t, int64(1010), value)
	})
}
