package utils

import (
	"time"
	"math/rand"
	"sync"
	"os"
)

var RandLocker = &sync.Mutex{}
var utcLocation, _ = time.LoadLocation("UTC")

func Random (min, max int) int {
	// Need to lock this on smaller devices: https://github.com/golang/go/issues/3611
	RandLocker.Lock()
	defer RandLocker.Unlock()
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max - min) + min
}

func CreateDirectory(directoryPath string) error {
	return os.MkdirAll(directoryPath,0777)
}

func GetHumanReadableTime(timestamp int) string {
	if timestamp <= 0 {
		return ""
	} else {
		unitxTime := time.Unix(int64(timestamp), 0)
		return unitxTime.In(utcLocation).Format(time.RFC822)
	}
}