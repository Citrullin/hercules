package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestIncrementInt64By(t *testing.T) {
	s := &storage{key: testKey, value: []byte{0xf6, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}}

	balance, err := coding.IncrementInt64By(s, testKey, 1, false)
	require.NoError(t, err)
	assert.Equal(t, int64(124), balance)
}
