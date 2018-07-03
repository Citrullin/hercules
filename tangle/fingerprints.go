package tangle

import (
	"time"
	"sync"
)

const fingerprintTTL = time.Duration(10) * time.Second

var fingerprintsLocker = &sync.RWMutex{}
var fingerprints map[string]time.Time

func fingerprintsOnLoad() {
	fingerprints = make(map[string]time.Time)
}

func cleanupFingerprints () {
	fingerprintsLocker.Lock()
	defer fingerprintsLocker.Unlock()

	now := time.Now()
	var toRemove []string
	for key, t := range fingerprints {
		if now.Sub(t) >= fingerprintTTL {
			toRemove = append(toRemove, key)
		}
	}
	for _, key := range toRemove {
		delete(fingerprints, key)
	}
}

func hasFingerprint (key []byte) bool {
	fingerprintsLocker.RLock()
	defer fingerprintsLocker.RUnlock()
	_, ok := fingerprints[string(key)]
	return ok
}

func addFingerprint (key []byte) {
	fingerprintsLocker.Lock()
	defer fingerprintsLocker.Unlock()
	fingerprints[string(key)] = time.Now()
}
