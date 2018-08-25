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

func TestPutBytes(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t,
		e.db.PutBytes(testKey, testValue, nil))
}

func TestGetBytes(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t,
		e.db.PutBytes(testKey, testValue, nil))

	value, err := e.db.GetBytes(testKey)
	require.NoError(t, err)

	assert.Equal(t, "value", string(value))
}

func TestHas(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t,
		e.db.PutBytes(testKey, testValue, nil))

	assert.True(t, e.db.Has(testKey))
}

func TestPut(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t,
		e.db.Put(testKey, 123, nil))

	value, err := e.db.GetBytes(testKey)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x4, 0x4, 0x0, 0xff, 0xf6}, value)
}

func TestGet(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t,
		e.db.PutBytes(testKey, []byte{0x4, 0x4, 0x0, 0xff, 0xf6}, nil))

	var value int
	err := e.db.Get(testKey, &value)
	require.NoError(t, err)
	assert.Equal(t, 123, value)
}

func TestRemove(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t,
		e.db.PutBytes(testKey, testValue, nil))

	require.NoError(t,
		e.db.Remove(testKey))

	assert.False(t, e.db.Has(testKey))
}

func TestRemovePrefix(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t, e.db.PutBytes([]byte("abc"), testValue, nil))
	require.NoError(t, e.db.PutBytes([]byte("aac"), testValue, nil))

	require.NoError(t,
		e.db.RemovePrefix([]byte("aa")))

	assert.True(t, e.db.Has([]byte("abc")))
	assert.False(t, e.db.Has([]byte("aac")))
}

func TestRemoveKeysFromCategoryBefore(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	key := db.AsKey(testKey, db.KEY_EDGE)
	require.NoError(t, e.db.Put(key, int64(1000), nil))

	assert.Equal(t, 1, e.db.RemoveKeysFromCategoryBefore(db.KEY_EDGE, 2000))

	assert.False(t, e.db.Has(key))
}

func TestCountPrefix(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t, e.db.PutBytes([]byte("abc"), testValue, nil))
	require.NoError(t, e.db.PutBytes([]byte("aac"), testValue, nil))

	assert.Equal(t, 2, e.db.CountPrefix([]byte("a")))
}

func TestIncrementBy(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t, e.db.Put(testKey, int64(1000), nil))

	value, err := e.db.IncrementBy(testKey, 10, true)
	require.NoError(t, err)

	assert.Equal(t, int64(1010), value)

	value, err = e.db.GetInt64(testKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1010), value)
}
