package transaction

import (
	"bytes"

	"../convert"
	"../crypt"
)

const (
	TOTAL_IOTAS int64 = 2779530283277761
)

func IsValidBundleTrytes(trytes []string) bool {
	// Get transaction objects
	var txs []*FastTX
	for _, tr := range trytes {
		t := convert.TrytesToTrits(tr)
		b := convert.TrytesToBytes(tr)
		if t == nil {
			return false
		}
		tx := TritsToFastTX(&t, b)
		txs = append(txs, tx)
	}
	return IsValidBundle(txs)
}

func IsValidBundle(txs []*FastTX) bool {
	// TODO: catch error, return false

	// Get transaction objects
	var trits = make(map[int][]int)
	for _, tx := range txs {
		t := convert.BytesToTrits(tx.Bytes)
		if t == nil {
			return false
		}
		trits[tx.CurrentIndex] = t
	}

	var value int64 = 0

	// Order by index
	var otxs []*FastTX
	current := 0
	for i := 0; i < len(txs); i++ {
		for _, tx := range txs {
			if current == tx.CurrentIndex {
				otxs = append(otxs, tx)
				value += tx.Value
				current++

				if value != 0 {
					if convert.BytesToTrits(tx.Address)[:243][242] != 0 {
						return false
					}
					if value < -TOTAL_IOTAS || value > TOTAL_IOTAS {
						return false
					}
				}

			}
		}
	}
	// Fail if order of indexes not correct or bundle value not zero
	if len(otxs) != len(txs) || value != 0 {
		return false
	}

	// Create bundle hash from all transaction essences
	var kerl = new(crypt.Kerl)
	var bundleHash = make([]int, crypt.HASH_LENGTH)
	kerl.Initialize()
	for _, tx := range otxs {
		t := trits[tx.CurrentIndex]
		essence := GetEssenceTrits(&t)
		kerl.Absorb(essence, 0, len(essence))

	}
	kerl.Squeeze(bundleHash, 0, crypt.HASH_LENGTH)
	for _, tx := range otxs {
		if !bytes.Equal(tx.Bundle, convert.TritsToBytes(bundleHash)) {
			return false
		}
	}

	normalizedBundleHash := NormalizedBundle(bundleHash)
	i := len(otxs)
	for i = 0; i < len(otxs); i++ {
		tx := otxs[i]
		if tx.Value < 0 {
			var kerl = new(crypt.Kerl)
			kerl.Initialize()
			address := tx.Address
			offset := 0
			offsetNext := 0
			for bytes.Equal(address, tx.Address) {
				offsetNext = (offset+NUMBER_OF_FRAGMENT_CHUNKS-1)%(crypt.HASH_LENGTH/3) + 1
				t := trits[tx.CurrentIndex]
				digestTrits := Digest(normalizedBundleHash, t, offset%81, 0, true)

				kerl.Absorb(digestTrits, 0, crypt.HASH_LENGTH)
				i++
				if i == len(otxs) {
					break
				}
				tx = otxs[i]
				if tx.Value != 0 {
					break
				}
				offset = offsetNext
			}
			// Verify signature
			var addressTrits = make([]int, crypt.HASH_LENGTH)
			kerl.Squeeze(addressTrits, 0, crypt.HASH_LENGTH)
			if !bytes.Equal(address, convert.TritsToBytes(addressTrits)[:49]) {
				return false
			}
		}
	}

	return true
}
