package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestForPrefixInt64(t *testing.T) {
	s := &storage{key: []byte("abc"), value: []byte{0x04, 0x04, 0x00, 0xff, 0xf6}}

	require.NoError(t, coding.ForPrefixInt64(s, []byte("a"), false, func(key []byte, value int64) (bool, error) {
		assert.Equal(t, []byte("abc"), key)
		assert.Equal(t, int64(123), value)
		return true, nil
	}))
}

func TestForPrefixInt(t *testing.T) {
	s := &storage{key: []byte("abc"), value: []byte{0x04, 0x04, 0x00, 0xff, 0xf6}}

	require.NoError(t, coding.ForPrefixInt(s, []byte("a"), false, func(key []byte, value int) (bool, error) {
		assert.Equal(t, []byte("abc"), key)
		assert.Equal(t, 123, value)
		return true, nil
	}))
}

func TestForPrefixBytes(t *testing.T) {
	s := &storage{key: []byte("abc"), value: []byte{0x06, 0x0a, 0x00, 0x03, 0x01, 0x02, 0x03}}

	require.NoError(t, coding.ForPrefixBytes(s, []byte("a"), false, func(key, value []byte) (bool, error) {
		assert.Equal(t, []byte("abc"), key)
		assert.Equal(t, []byte{1, 2, 3}, value)
		return true, nil
	}))
}
