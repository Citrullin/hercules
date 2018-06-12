package utils

import (
	"time"
	"math/rand"
	"sync"
	"os"
)

var RandLocker = &sync.Mutex{}

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