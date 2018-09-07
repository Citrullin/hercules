package ns_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestRemove(t *testing.T) {
	s := &storage{key: testKey, value: testValue}

	require.NoError(t, ns.Remove(s, 'k'))

	assert.Nil(t, s.key)
	assert.Nil(t, s.value)
}
