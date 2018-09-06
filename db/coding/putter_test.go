package coding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"."
)

func TestPutBool(t *testing.T) {
	s := &storage{}

	require.NoError(t, coding.PutBool(s, testKey, false))
	assert.Equal(t, []byte{0x03, 0x02, 0x00, 0x00}, s.value)

	require.NoError(t, coding.PutBool(s, testKey, true))
	assert.Equal(t, []byte{0x03, 0x02, 0x00, 0x01}, s.value)
}

func TestPutInt(t *testing.T) {
	s := &storage{}

	require.NoError(t, coding.PutInt(s, testKey, 123))
	assert.Equal(t, []byte{0x04, 0x04, 0x00, 0xff, 0xf6}, s.value)
}

func TestPutInt64(t *testing.T) {
	s := &storage{}

	require.NoError(t, coding.PutInt64(s, testKey, 123))
	assert.Equal(t, []byte{0x04, 0x04, 0x00, 0xff, 0xf6}, s.value)
}

func TestPutString(t *testing.T) {
	s := &storage{}

	require.NoError(t, coding.PutString(s, testKey, "test"))
	assert.Equal(t, []byte{0x07, 0x0c, 0x00, 0x04, 0x74, 0x65, 0x73, 0x74}, s.value)
}

func BenchmarkPutBool(b *testing.B) {
	s := &storage{}
	value := true

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		coding.PutBool(s, testKey, value)
	}
	b.StopTimer()

	assert.Equal(b, []byte{0x03, 0x02, 0x00, 0x01}, s.value)
}

func BenchmarkPutInt(b *testing.B) {
	s := &storage{}
	value := 123

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		coding.PutInt(s, testKey, value)
	}
	b.StopTimer()

	assert.Equal(b, []byte{0x04, 0x04, 0x00, 0xff, 0xf6}, s.value)
}

func BenchmarkPutInt64(b *testing.B) {
	s := &storage{}
	value := int64(123)

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		coding.PutInt64(s, testKey, value)
	}
	b.StopTimer()

	assert.Equal(b, []byte{0x04, 0x04, 0x00, 0xff, 0xf6}, s.value)
}

func BenchmarkPutString(b *testing.B) {
	s := &storage{}
	value := "test"

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		coding.PutString(s, testKey, value)
	}
	b.StopTimer()

	assert.Equal(b, []byte{0x07, 0x0c, 0x00, 0x04, 0x74, 0x65, 0x73, 0x74}, s.value)
}
