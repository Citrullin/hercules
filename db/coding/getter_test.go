package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestGetBool(t *testing.T) {
	value, err := coding.GetBool(&storage{key: testKey, value: []byte{0x03, 0x02, 0x00, 0x00}}, testKey)
	require.NoError(t, err)
	assert.False(t, value)

	value, err = coding.GetBool(&storage{key: testKey, value: []byte{0x03, 0x02, 0x00, 0x01}}, testKey)
	require.NoError(t, err)
	assert.True(t, value)
}

func TestGetInt64(t *testing.T) {
	s := &storage{key: testKey, value: []byte{0x04, 0x04, 0x00, 0xff, 0xf6}}

	value, err := coding.GetInt64(s, testKey)
	require.NoError(t, err)
	assert.Equal(t, int64(123), value)
}

func TestGetString(t *testing.T) {
	s := &storage{key: testKey, value: []byte{0x07, 0x0c, 0x00, 0x04, 0x74, 0x65, 0x73, 0x74}}

	value, err := coding.GetString(s, testKey)
	require.NoError(t, err)
	assert.Equal(t, "test", value)
}