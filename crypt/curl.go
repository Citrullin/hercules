package crypt

import (
	"math"
)

const (
	NUMBER_OF_ROUNDSP27 = 27
	NUMBER_OF_ROUNDSP81 = 81
	HASH_LENGTH         = 243
	STATE_LENGTH        = 3 * HASH_LENGTH
)

var (
	TRUTH_TABLE = []int{1, 0, -1, 2, 1, -1, 0, 2, -1, 1, 0}
)

type Hash interface {
	Initialize()
	InitializeCurl(trits []int, length int, rounds int)
	Reset()
	Absorb(trits []int, offset int, length int)
	Squeeze(resp []int, offset int, length int) []int
}

type Curl struct {
	Hash
	state  []int
	rounds int
}

func (curl *Curl) Initialize() {
	curl.InitializeCurl(nil, 0, curl.rounds)
}

func (curl *Curl) InitializeCurl(trits []int, length int, rounds int) {
	curl.rounds = rounds
	if trits != nil {
		curl.state = trits
	} else {
		curl.state = make([]int, STATE_LENGTH)
	}
}

func (curl *Curl) Reset() {
	curl.InitializeCurl(nil, 0, curl.rounds)
}

func (curl *Curl) Absorb(trits []int, offset int, length int) {
	for {
		limit := int(math.Min(HASH_LENGTH, float64(length)))
		copy(curl.state, trits[offset:offset+limit])
		curl.Transform()
		offset += HASH_LENGTH
		length -= HASH_LENGTH
		if length <= 0 {
			break
		}
	}
}

func (curl *Curl) Squeeze(resp []int, offset int, length int) []int {
	for {
		limit := int(math.Min(HASH_LENGTH, float64(length)))
		copy(resp[offset:offset+limit], curl.state)
		curl.Transform()
		offset += HASH_LENGTH
		length -= HASH_LENGTH
		if length <= 0 {
			break
		}
	}
	return resp
}

func (curl *Curl) Transform() {
	var index = 0
	for round := 0; round < curl.rounds; round++ {
		stateCopy := make([]int, STATE_LENGTH)
		copy(stateCopy, curl.state)
		for i := 0; i < STATE_LENGTH; i++ {
			incr := 364
			if index >= 365 {
				incr = -365
			}
			index2 := index + incr
			curl.state[i] = TRUTH_TABLE[stateCopy[index]+(stateCopy[index2]<<2)+5]
			index = index2
		}
	}
}

func RunHashCurl(trits []int) []int {
	var curl = new(Curl)
	var resp = make([]int, HASH_LENGTH)

	curl.InitializeCurl(nil, 0, NUMBER_OF_ROUNDSP81)
	curl.Absorb(trits, 0, len(trits))
	curl.Squeeze(resp, 0, HASH_LENGTH)

	return resp
}
