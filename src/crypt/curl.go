package crypt

import "math"

const NUMBER_OF_ROUNDS = 81
const HASH_LENGTH = 243
const STATE_LENGTH = 3 * HASH_LENGTH

var truthTable = []int{1, 0, -1, 2, 1, -1, 0, 2, -1, 1, 0}

func Curl (trits []int) []int {
	state := make([]int, STATE_LENGTH)

	for length, offset := len(trits), 0; length > 0; length = length - HASH_LENGTH {
		limit := int(math.Min(HASH_LENGTH, float64(length)))
		for i := 0; i < limit ; i, offset = i + 1, offset + 1 {
			state[i] = trits[offset]
		}
		transform(&state)
	}

	return squeeze(&state,243)
}


func transform (state *[]int) {
	var index = 0
	for round := 0; round < NUMBER_OF_ROUNDS; round++ {
		stateCopy := make([]int, STATE_LENGTH)
		copy(stateCopy, *state)
		for i := 0; i < STATE_LENGTH; i++ {
			incr := 364
			if index >= 365 {
				incr = -365
			}
			index2 := index + incr
			(*state)[i] = truthTable[stateCopy[index] + (stateCopy[index2] << 2) + 5]
			index = index2
		}
	}
}

func squeeze (state *[]int, length int) []int {
	resp := make([]int, length)
	for offset := 0; length > 0; length = length - HASH_LENGTH {
		limit := int(math.Min(HASH_LENGTH, float64(length)))
		for i := 0; i < limit ; i, offset = i + 1, offset + 1 {
			resp[offset] = (*state)[i]
		}
		transform(state)
	}
	return resp
}