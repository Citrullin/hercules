package convert

import "math"

var (
	TRYTES           = "NOPQRSTUVWXYZ9ABCDEFGHIJKLM"
	trytesToTritsMap = map[rune][]int{
		'9': []int{0, 0, 0},
		'A': []int{1, 0, 0},
		'B': []int{-1, 1, 0},
		'C': []int{0, 1, 0},
		'D': []int{1, 1, 0},
		'E': []int{-1, -1, 1},
		'F': []int{0, -1, 1},
		'G': []int{1, -1, 1},
		'H': []int{-1, 0, 1},
		'I': []int{0, 0, 1},
		'J': []int{1, 0, 1},
		'K': []int{-1, 1, 1},
		'L': []int{0, 1, 1},
		'M': []int{1, 1, 1},
		'N': []int{-1, -1, -1},
		'O': []int{0, -1, -1},
		'P': []int{1, -1, -1},
		'Q': []int{-1, 0, -1},
		'R': []int{0, 0, -1},
		'S': []int{1, 0, -1},
		'T': []int{-1, 1, -1},
		'U': []int{0, 1, -1},
		'V': []int{1, 1, -1},
		'W': []int{-1, -1, 0},
		'X': []int{0, -1, 0},
		'Y': []int{1, -1, 0},
		'Z': []int{-1, 0, 0},
	}
)

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
		pos := index(i*3+0) + (index(i*3+1))*3 + (index(i*3+2))*9 + 13
		trytes += string(CharCodeAt(TRYTES, pos))
	}

	return trytes
}

func TrytesToTrits(trytes string) (trits []int) {
	for _, char := range trytes {
		mappedTrits, charIsTryte := trytesToTritsMap[char]
		if !charIsTryte {
			return nil
		}

		trits = append(trits, mappedTrits...)
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

func IsTrytes(trytes string, length int) bool {
	if len(trytes) != length {
		return false
	}

	for _, char := range trytes {
		_, charIsTryte := trytesToTritsMap[char]
		if !charIsTryte {
			return false
		}
	}

	return true
}
