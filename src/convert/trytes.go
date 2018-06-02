package convert

import "unicode/utf8"
import (
	"math"
)

var TRYTES = "NOPQRSTUVWXYZ9ABCDEFGHIJKLM"
var TRYTES_TO_TRITS = []int{
	0, 0, 0,
	1, 0, 0,
	-1, 1, 0,
	0, 1, 0,
	1, 1, 0,
	-1, -1, 1,
	0, -1, 1,
	1, -1, 1,
	-1, 0, 1,
	0, 0, 1,
	1, 0, 1,
	-1, 1, 1,
	0, 1, 1,
	1, 1, 1,
	-1, -1, -1,
	0, -1, -1,
	1, -1, -1,
	-1, 0, -1,
	0, 0, -1,
	1, 0, -1,
	-1, 1, -1,
	0, 1, -1,
	1, 1, -1,
	-1, -1, 0,
	0, -1, 0,
	1, -1, 0,
	-1, 0, 0,
}

func TritsToTrytes(trits []int) string {
	l := len(trits)
	size := int(math.Ceil(float64(len(trits)) / 3))

	index := func(i int) int {
		if i >= l {
			return 0
		}
		return trits[i]
	}

	trytes := ""

	for i := 0; i < size; i += 1 {
		pos := index(i*3+0) + (index(i*3 + 1))*3 + (index(i*3 + 2))*9 + 13
		trytes += string(CharCodeAt(TRYTES, pos))
	}

	return trytes
}

func TrytesToTrits(trytes string) (trits []int) {
	defer func() {
		if r := recover(); r != nil {
			trits = nil
		}
	}()

	var k int

	size := utf8.RuneCountInString(trytes)
	trits = make([]int, size*3)

	for i, j := 0, 0; i < size; i, j = i+1, j+3 {
		char := int(CharCodeAt(trytes, i))
		k = (char - 64) * 3

		if k < 0 {
			k = 0
		}

		trits[j+0] = TRYTES_TO_TRITS[k+0]
		trits[j+1] = TRYTES_TO_TRITS[k+1]
		trits[j+2] = TRYTES_TO_TRITS[k+2]
	}

	return trits
}

func TrytesToBytes(trytes string) []byte {
	return TritsToBytes(TrytesToTrits(trytes))
}

func BytesToTrytes(bytes []byte) string {
	return TritsToTrytes(BytesToTrits(bytes))
}

func CharCodeAt(s string, n int) rune {
	i := 0
	for _, r := range s {
		if i == n {
			return r
		}
		i++
	}
	return 0
}


func IsTrytes (trytes string, length int) bool {
	if len(trytes) != length || TrytesToTrits(trytes) == nil {
		return false
	}
	return true
}