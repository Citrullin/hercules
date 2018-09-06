package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestHasKeyInCategoryWithInt64LowerEqual(t *testing.T) {
	s := &storage{key: []byte("abc"), value: []byte{0x04, 0x04, 0x00, 0xff, 0xf6}}

	assert.True(t, coding.HasKeyInCategoryWithInt64LowerEqual(s, 'a', 124))
}

func TestRemoveKeysInCategoryWithInt64LowerEqual(t *testing.T) {
	s := &storage{key: []byte("abc"), value: []byte{0x04, 0x04, 0x00, 0xff, 0xf6}}

	require.Equal(t, 1, coding.RemoveKeysInCategoryWithInt64LowerEqual(s, 'a', 124))

	assert.Nil(t, s.key)
	assert.Nil(t, s.value)
}

func TestSumInt64InCategory(t *testing.T) {
	s := &storage{key: []byte("abc"), value: []byte{0x04, 0x04, 0x00, 0xff, 0xf6}}

	assert.Equal(t, int64(123), coding.SumInt64InCategory(s, 'a'))
}

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
