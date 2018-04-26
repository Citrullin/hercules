package crypt

func IsValidPoW (trits []int, mwm int) bool {
	hash := Curl(trits[:8019])
	for i := len(hash) - mwm; i < len(hash); i++ {
		if hash[i] != 0 { return false }
	}
	return true
}
