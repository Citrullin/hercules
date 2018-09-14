package coding_test

import (
	"crypto/md5"
	"crypto/sha256"
	"testing"

	"github.com/cespare/xxhash"
	"github.com/minio/highwayhash"
)

func BenchmarkMD5(b *testing.B) {

	bytes := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01}
	var res [16]byte

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		res = md5.Sum(bytes)
	}
	b.StopTimer()
	_ = res
}

func BenchmarkSHA256(b *testing.B) {

	bytes := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01}
	var res [32]byte

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		res = sha256.Sum256(bytes)
	}
	b.StopTimer()
	_ = res
}

func BenchmarkHighwayHash(b *testing.B) {

	bytes := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01}
	key := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x10, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01}
	var res [16]byte

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		res = highwayhash.Sum128(bytes, key)
	}
	b.StopTimer()
	_ = res
}

func BenchmarkXXHash(b *testing.B) {

	bytes := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01}
	var res uint64

	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		res = xxhash.Sum64(bytes)
	}
	b.StopTimer()
	_ = res
}
