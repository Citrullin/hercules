package transaction

import (
	"strings"

	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/crypt"
)

const (
	HASH_LENGTH                 = crypt.HASH_LENGTH
	NUMBER_OF_KEYS_IN_MILESTONE = 20
	NUMBER_OF_FRAGMENT_CHUNKS   = 27
	FRAGMENT_LENGTH             = HASH_LENGTH * NUMBER_OF_FRAGMENT_CHUNKS
	NUMBER_OF_SECURITY_LEVELS   = 3

	TRYTE_WIDTH                = 3
	MIN_TRYTE_VALUE            = -13
	MAX_TRYTE_VALUE            = 13
	NORMALIZED_FRAGMENT_LENGTH = HASH_LENGTH / TRYTE_WIDTH / NUMBER_OF_SECURITY_LEVELS
)

var (
	NULL_HASH_TRITS = convert.TrytesToTrits(strings.Repeat("9", 81))
)

func Address(digests []int) []int {
	if len(digests) == 0 || len(digests)%HASH_LENGTH != 0 {
		panic("Invalid digests length")
	}
	address := make([]int, HASH_LENGTH)
	hash := new(crypt.Curl)
	hash.InitializeCurl(nil, 0, crypt.NUMBER_OF_ROUNDSP27)
	hash.Absorb(digests, 0, len(digests))
	hash.Squeeze(address, 0, len(address))
	return address
}

func Digest(normalizedBundleFragment []int, signatureFragment []int, nbOffset int, sfOffset int, asKerl bool) []int {
	if len(normalizedBundleFragment)%NORMALIZED_FRAGMENT_LENGTH != 0 {
		panic("Invalid normalized bundleValidator fragment length!")
	}
	if len(signatureFragment) != FRAGMENT_LENGTH {
		//panic("Invalid signature fragment length!")
	}
	digest := make([]int, HASH_LENGTH)
	buffer := make([]int, FRAGMENT_LENGTH)
	copy(buffer, signatureFragment[sfOffset:sfOffset+FRAGMENT_LENGTH])
	var hsh crypt.Hash
	if asKerl {
		hsh = new(crypt.Kerl)
		hsh.Initialize()
	} else {
		hsh = new(crypt.Curl)
		hsh.InitializeCurl(nil, 0, crypt.NUMBER_OF_ROUNDSP27)
	}

	for j := 0; j < NUMBER_OF_FRAGMENT_CHUNKS; j++ {
		for k := normalizedBundleFragment[nbOffset+j] - MIN_TRYTE_VALUE; k > 0; k-- {
			hsh.Reset()
			hsh.Absorb(buffer, j*HASH_LENGTH, HASH_LENGTH)
			hsh.Squeeze(buffer, j*HASH_LENGTH, HASH_LENGTH)
		}
	}

	hsh.Reset()
	hsh.Absorb(buffer, 0, FRAGMENT_LENGTH)
	hsh.Squeeze(digest, 0, HASH_LENGTH)

	return digest
}

func NormalizedBundle(bundle []int) []int {
	if len(bundle) != HASH_LENGTH {
		panic("Invalid bundleValidator length")
	}

	normalizedBundle := make([]int, HASH_LENGTH/TRYTE_WIDTH)
	for i := 0; i < NUMBER_OF_SECURITY_LEVELS; i++ {
		sum := 0
		for j := i * NORMALIZED_FRAGMENT_LENGTH; j < (i+1)*NORMALIZED_FRAGMENT_LENGTH; j++ {
			normalizedBundle[j] = bundle[j*TRYTE_WIDTH] + bundle[j*TRYTE_WIDTH+1]*3 + bundle[j*TRYTE_WIDTH+2]*9
			sum += normalizedBundle[j]
		}
		if sum > 0 {
			for sum > 0 {
				sum--
				for j := i * NORMALIZED_FRAGMENT_LENGTH; j < (i+1)*NORMALIZED_FRAGMENT_LENGTH; j++ {
					if normalizedBundle[j] > MIN_TRYTE_VALUE {
						normalizedBundle[j]--
						break
					}
				}
			}
		} else {
			for sum < 0 {
				sum++
				for j := i * NORMALIZED_FRAGMENT_LENGTH; j < (i+1)*NORMALIZED_FRAGMENT_LENGTH; j++ {
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

func GetMerkleRoot(hash []int, trits []int, offset int, indexIn int, size int) []int {
	index := uint32(indexIn)
	curl := new(crypt.Curl)
	curl.InitializeCurl(nil, 0, crypt.NUMBER_OF_ROUNDSP27)
	for i := 0; i < size; i++ {
		curl.Reset()
		if index&1 == 0 {
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
