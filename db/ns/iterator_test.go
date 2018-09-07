package ns_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestForNamespace(t *testing.T) {
	s := &storage{key: testKey, value: testValue}

	count := 0
	require.NoError(t, ns.ForNamespace(s, 'k', true, func(key, value []byte) (bool, error) {
		assert.Equal(t, testKey, key)
		assert.Equal(t, testValue, value)
		count++
		return true, nil
	}))
	assert.Equal(t, 1, count)
}
