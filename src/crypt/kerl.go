package crypt

import (
	"github.com/tonnerre/golang-go.crypto/sha3"
	"convert"
	"hash"
	"math/big"
)

const BIT_HASH_LENGTH = 384
const BYTE_HASH_LENGTH = BIT_HASH_LENGTH / 8

var one = big.NewInt(1)
var minusOne = big.NewInt(-1)

type Kerl struct {
	Hash
	byte_state []byte
	trit_state []int
	hash hash.Hash
}

func (kerl *Kerl) Initialize() {
	kerl.hash = sha3.NewKeccak384()
	kerl.byte_state = make([]byte, BYTE_HASH_LENGTH)
	kerl.trit_state = make([]int, HASH_LENGTH)
}

func (kerl *Kerl) Reset() {
	kerl.Initialize()
}

func (kerl *Kerl) Absorb(trits []int, offset int, length int) {
	if length % 243 != 0 { panic("wrong length provided for kerl") }
	for {
		copy(kerl.trit_state[:HASH_LENGTH], trits[offset:offset+HASH_LENGTH])
		kerl.trit_state[HASH_LENGTH - 1] = 0
		IntToBytes(convert.TritsToInt(kerl.trit_state), kerl.byte_state, 0)
		kerl.hash.Write(kerl.byte_state)
		offset += HASH_LENGTH
		length -= HASH_LENGTH
		if length <= 0 { break }
	}
}

func (kerl *Kerl) Squeeze(trits []int, offset int, length int) []int {
	if length % 243 != 0 { panic("wrong length provided for kerl") }
	for {
		kerl.byte_state = kerl.hash.Sum(nil)
		intValue := BytesToInt(kerl.byte_state, 0 , BYTE_HASH_LENGTH)
		kerl.trit_state = convert.IntToTrits(intValue, len(kerl.trit_state))
		kerl.trit_state[HASH_LENGTH - 1] = 0
		copy(trits[offset: offset + HASH_LENGTH], kerl.trit_state[0:HASH_LENGTH])

		i := len(kerl.byte_state) - 1
		for i >= 0 {
			kerl.byte_state[i] = kerl.byte_state[i] ^ 0xFF
			i--
		}
		kerl.hash.Write(kerl.byte_state)

		offset += HASH_LENGTH
		length -= HASH_LENGTH
		if length <= 0 { break }
	}
	return trits
}

func RunHashKerl(trits []int) []int {
	var kerl = new(Kerl)
	var resp = make([]int, HASH_LENGTH)

	kerl.Initialize()
	kerl.Absorb(trits, 0, len(trits))
	kerl.Squeeze(resp, 0, HASH_LENGTH)

	return resp
	return nil
}

func BytesToInt (input []byte, offset int, size int) *big.Int {
	var cp = make([]byte, len(input))
	copy(cp, input)
	isPositive := cp[0] >> 7 == 0
	nullEndian := cp [len(cp)-1] == 0
	if !isPositive {
		for i, b := range cp {
			//if i != len(cp)- 1 {
				cp[i] = b ^0xFF
			//}
		}
		if !nullEndian {
			cp [len(cp)-1] += 1
		}
	}
	bigInt := big.NewInt(0).SetBytes(cp[offset:offset+size])
	if !isPositive {
		if nullEndian {
			bigInt = bigInt.Add(bigInt, one)
		}
		bigInt = bigInt.Mul(bigInt, minusOne)
	}
	return bigInt
}

func IntToBytes (value *big.Int, destination []byte, offset int) {
	if len(destination) - offset < BYTE_HASH_LENGTH { panic("Destination array has invalid size for Kerl") }
	bts := value.Bytes()
	isPositive := value.Sign() >= 0

	i := 0
	for i + len(bts) < BYTE_HASH_LENGTH {
		if isPositive {
			destination[i] = 0
		} else {
			destination[i] = 255
		}
		i++
	}
	j := len(bts)
	for j > 0 {
		destination[i] = bts[len(bts) - j]
		if isPositive {
			destination[i] = bts[len(bts) - j]
		} else {
			destination[i] = bts[len(bts) - j] ^ 0xFF
		}
		j--
		i++
	}
	if !isPositive {
		for j = len(destination) - 1; j >= 0; j-- {
			destination[j] += 1
			if destination[j] != 0 { break }
		}
	}
}