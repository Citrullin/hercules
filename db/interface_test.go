package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
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

	t.Run("Has", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.PutBytes(testKey, testValue, nil))

		assert.True(t, e.db.Has(testKey))
	})

	t.Run("Put", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t,
			e.db.Put(testKey, 123, nil))

		value, err := e.db.GetBytes(testKey)
		require.NoError(t, err)
		assert.Equal(t, []byte{0x4, 0x4, 0x0, 0xff, 0xf6}, value)
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

		assert.False(t, e.db.Has(testKey))
	})

	t.Run("RemovePrefix", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		require.NoError(t, e.db.PutBytes([]byte("abc"), testValue, nil))
		require.NoError(t, e.db.PutBytes([]byte("aac"), testValue, nil))

		require.NoError(t,
			e.db.RemovePrefix([]byte("aa")))

		assert.True(t, e.db.Has([]byte("abc")))
		assert.False(t, e.db.Has([]byte("aac")))
	})

	t.Run("RemoveKeysFromCategoryBefore", func(t *testing.T) {
		e := setUpTestEnvironment(t, setUpFn)
		defer e.tearDown()

		key := db.AsKey(testKey, db.KEY_EDGE)
		require.NoError(t, e.db.Put(key, int64(1000), nil))

		assert.Equal(t, 1, e.db.RemoveKeysFromCategoryBefore(db.KEY_EDGE, 2000))

		assert.False(t, e.db.Has(key))
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

		require.NoError(t, e.db.Put(testKey, int64(1000), nil))

		value, err := e.db.IncrementBy(testKey, 10, true)
		require.NoError(t, err)

		assert.Equal(t, int64(1010), value)

		value, err = e.db.GetInt64(testKey)
		require.NoError(t, err)
		assert.Equal(t, int64(1010), value)
	})
}
