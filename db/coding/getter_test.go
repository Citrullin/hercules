package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestGetBool(t *testing.T) {
	value, err := coding.GetBool(&storage{key: testKey, value: []byte{0x00}}, testKey)
	require.NoError(t, err)
	assert.False(t, value)

	value, err = coding.GetBool(&storage{key: testKey, value: []byte{0x01}}, testKey)
	require.NoError(t, err)
	assert.True(t, value)
}

func TestGetInt(t *testing.T) {
	s := &storage{key: testKey, value: []byte{0xf6, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}}

	value, err := coding.GetInt(s, testKey)
	require.NoError(t, err)
	assert.Equal(t, 123, value)
}

func TestGetInt64(t *testing.T) {
	s := &storage{key: testKey, value: []byte{0xf6, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}}

	value, err := coding.GetInt64(s, testKey)
	require.NoError(t, err)
	assert.Equal(t, int64(123), value)
}

func TestGetString(t *testing.T) {
	s := &storage{key: testKey, value: []byte{0x74, 0x65, 0x73, 0x74}}

	value, err := coding.GetString(s, testKey)
	require.NoError(t, err)
	assert.Equal(t, "test", value)
}

func BenchmarkGetBool(b *testing.B) {
	s := &storage{key: testKey, value: []byte{0x01}}
	value := false
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		value, _ = coding.GetBool(s, testKey)
	}
	b.StopTimer()
	assert.True(b, value)
}

func BenchmarkGetInt(b *testing.B) {
	s := &storage{key: testKey, value: []byte{0xf6, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}}
	value := 0
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		value, _ = coding.GetInt(s, testKey)
	}
	b.StopTimer()
	assert.Equal(b, 123, value)
}

func BenchmarkGetInt64(b *testing.B) {
	s := &storage{key: testKey, value: []byte{0xf6, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}}
	value := int64(0)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		value, _ = coding.GetInt64(s, testKey)
	}
	b.StopTimer()
	assert.Equal(b, int64(123), value)
}

func BenchmarkGetString(b *testing.B) {
	s := &storage{key: testKey, value: []byte{0x74, 0x65, 0x73, 0x74}}
	value := ""
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		value, _ = coding.GetString(s, testKey)
	}
	b.StopTimer()
	assert.Equal(b, "test", value)
}
