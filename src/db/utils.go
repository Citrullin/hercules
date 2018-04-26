package db

import (
	"time"
	"math/rand"
)

func random (min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max - min) + min
}
