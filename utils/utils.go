package utils

import (
	"math/rand"
	"os"
	"sync"
	"time"
)

var rng *rand.Rand
var randLock = &sync.Mutex{}

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func Random(min, max int) int {
	// Need to lock this on smaller devices: https://github.com/golang/go/issues/3611
	randLock.Lock()
	defer randLock.Unlock()
	return rng.Intn(max-min) + min
}

func CreateDirectory(directoryPath string) error {
	return os.MkdirAll(directoryPath, 0777)
}

func GetHumanReadableTime(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	} else {
		unitxTime := time.Unix(timestamp, 0)
		return unitxTime.In(time.UTC).Format(time.RFC822)
	}
}
