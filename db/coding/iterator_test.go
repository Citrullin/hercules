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
