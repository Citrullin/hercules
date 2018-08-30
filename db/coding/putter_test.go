package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestPutBool(t *testing.T) {
	p := &putter{}

	require.NoError(t, coding.PutBool(p, testKey, false))
	assert.Equal(t, []byte{0x03, 0x02, 0x00, 0x00}, p.value)

	require.NoError(t, coding.PutBool(p, testKey, true))
	assert.Equal(t, []byte{0x03, 0x02, 0x00, 0x01}, p.value)
}
