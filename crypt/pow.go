package crypt

import "gitlab.com/semkodev/hercules/convert"

func IsValidPoW (hsh []byte, mwm int) bool {
	hash := convert.BytesToTrits(hsh)
	for i := len(hash) - mwm; i < len(hash); i++ {
		if hash[i] != 0 { return false }
	}
	return true
}
