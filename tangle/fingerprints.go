package tangle

import (
	"sync"
	"time"
)

const fingerprintTTL = time.Duration(10) * time.Second

var fingerprints map[string]time.Time
var fingerprintsLock = &sync.RWMutex{}

func fingerprintsOnLoad() {
	fingerprints = make(map[string]time.Time)
}

func cleanupFingerprints() {

	ttl := fingerprintTTL

	if lowEndDevice {
		ttl = ttl * 6
	}

	now := time.Now()
	var toRemove []string

	fingerprintsLock.RLock()
	for key, t := range fingerprints {
		if now.Sub(t) >= ttl {
			toRemove = append(toRemove, key)
		}
	}
	fingerprintsLock.RUnlock()

	fingerprintsLock.Lock()
	for _, key := range toRemove {
		delete(fingerprints, key)
	}
	fingerprintsLock.Unlock()
}

func hasFingerprint(key []byte) bool {
	fingerprintsLock.RLock()
	defer fingerprintsLock.RUnlock()
	_, ok := fingerprints[string(key)]
	return ok
}

func addFingerprint(key []byte) {
	fingerprintsLock.Lock()
	defer fingerprintsLock.Unlock()
	fingerprints[string(key)] = time.Now()
}
