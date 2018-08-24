package db_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPutBytes(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	err := e.db.PutBytes([]byte("test"), []byte("value"), nil)
	require.NoError(t, err)
}

func TestGetBytes(t *testing.T) {
	e := setUpTestEnvironment(t)
	defer e.tearDown()

	require.NoError(t, e.db.PutBytes([]byte("test"), []byte("value"), nil))

	value, err := e.db.GetBytes([]byte("test"))
	require.NoError(t, err)

	assert.Equal(t, "value", string(value))
}
