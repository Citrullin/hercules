package transaction

import (
	"strings"
	"convert"
	"crypt"
)

const HASH_LENGTH = crypt.HASH_LENGTH
const NUMBER_OF_KEYS_IN_MILESTONE = 20
const NUMBER_OF_FRAGMENT_CHUNKS = 27
const FRAGMENT_LENGTH = HASH_LENGTH * NUMBER_OF_FRAGMENT_CHUNKS
const NUMBER_OF_SECURITY_LEVELS = 3

const TRYTE_WIDTH = 3
const MIN_TRYTE_VALUE = -13
const MAX_TRYTE_VALUE = 13
const NORMALIZED_FRAGMENT_LENGTH = HASH_LENGTH / TRYTE_WIDTH / NUMBER_OF_SECURITY_LEVELS

var NULL_HASH_TRITS = convert.TrytesToTrits(strings.Repeat("9", 81))

func Address(digests []int) []int {
	if len(digests) == 0 || len(digests) % HASH_LENGTH != 0 {
		panic("Invalid digests length")
	}
	address := make([]int, HASH_LENGTH)
	hash := new(crypt.Curl)
	hash.Initialize(nil, 0, crypt.NUMBER_OF_ROUNDSP27)
	hash.Absorb(digests, 0, len(digests))
	hash.Squeeze(address, 0, len(address))
	return address
}

func Digest(normalizedBundleFragment []int, signatureFragment []int) []int {
	if len(normalizedBundleFragment) != NORMALIZED_FRAGMENT_LENGTH {
		panic("Invalid normalized bundleValidator fragment length!")
	}
	if len(signatureFragment) != FRAGMENT_LENGTH {
		panic("Invalid signature fragment length!")
	}
	digest := make([]int, HASH_LENGTH)
	buffer := make([]int, FRAGMENT_LENGTH)
	copy(buffer, signatureFragment)
	hash := new(crypt.Curl)
	hash.Initialize(nil, 0, crypt.NUMBER_OF_ROUNDSP27)

	for j := 0; j < NUMBER_OF_FRAGMENT_CHUNKS; j++ {
		for k := normalizedBundleFragment[j] - MIN_TRYTE_VALUE - 1; k >= 0; k-- {
			hash.Reset()
			hash.Absorb(buffer, j * HASH_LENGTH, HASH_LENGTH)
			hash.Squeeze(buffer, j * HASH_LENGTH, HASH_LENGTH)
		}
	}

	hash.Reset()
	hash.Absorb(buffer, 0, len(buffer))
	hash.Squeeze(digest, 0, len(digest))

	return digest
}

func NormalizedBundle (bundle []int) []int {
	if len(bundle) != HASH_LENGTH {
		panic("Invalid bundleValidator length")
	}

	normalizedBundle := make([]int, HASH_LENGTH / TRYTE_WIDTH)
	for i := 0; i < NUMBER_OF_SECURITY_LEVELS; i++ {
		sum := 0
		for j := i * NORMALIZED_FRAGMENT_LENGTH; j < (i + 1) * NORMALIZED_FRAGMENT_LENGTH; j++ {
			normalizedBundle[j] = bundle[j*TRYTE_WIDTH] + bundle[j*TRYTE_WIDTH+1]*3 + bundle[j*TRYTE_WIDTH+2]*9
			sum += normalizedBundle[j]
		}
		if sum > 0 {
			for sum > 0 {
				sum--
				for j := i * NORMALIZED_FRAGMENT_LENGTH; j < (i + 1) * NORMALIZED_FRAGMENT_LENGTH; j++ {
					if normalizedBundle[j] > MIN_TRYTE_VALUE {
						normalizedBundle[j]--
						break
					}
				}
			}
		} else {
			for sum < 0 {
				sum++
				for j := i * NORMALIZED_FRAGMENT_LENGTH; j < (i + 1) * NORMALIZED_FRAGMENT_LENGTH; j++ {
					if normalizedBundle[j] < MAX_TRYTE_VALUE {
						normalizedBundle[j]++
						break
					}
				}
			}
		}
	}
	return normalizedBundle
}

func GetMerkleRoot (hash []int, trits []int, offset int, indexIn int, size int) []int {
	index := uint32(indexIn)
	curl := new(crypt.Curl)
	curl.Initialize(nil, 0, crypt.NUMBER_OF_ROUNDSP27)
	for i := 0; i < size; i++ {
		curl.Reset()
		if index & 1 == 0 {
			curl.Absorb(hash, 0, len(hash))
			curl.Absorb(trits, offset+i*HASH_LENGTH, HASH_LENGTH)
		} else {
			curl.Absorb(trits, offset+i*HASH_LENGTH, HASH_LENGTH)
			curl.Absorb(hash, 0, len(hash))
		}
		curl.Squeeze(hash, 0, len(hash))
		index >>= 1
	}
	if index != 0 {
		return NULL_HASH_TRITS
	}
	return hash
}
