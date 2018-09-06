package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestIncrementInt64By(t *testing.T) {
	s := &storage{key: testKey, value: []byte{0x04, 0x04, 0x00, 0xff, 0xf6}}

	balance, err := coding.IncrementInt64By(s, testKey, 1, false)
	require.NoError(t, err)
	assert.Equal(t, int64(124), balance)
}
