package ns_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"."
)

func TestCount(t *testing.T) {
	s := &storage{key: testKey, value: testValue}

	assert.Equal(t, 1, ns.Count(s, 'k'))
}
