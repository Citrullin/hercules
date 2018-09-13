package tangle

import (
	"sync"
	"time"
)

const (
	fingerprintTTL = time.Duration(10) * time.Second
)

type Fingerprint struct {
	ReceiveTime time.Time
	ReceiveHash []byte
}

var (
	fingerprints     map[string]*Fingerprint
	fingerprintsLock = &sync.RWMutex{}
)

func fingerprintsOnLoad() {
	fingerprints = make(map[string]*Fingerprint)
}

func cleanupFingerprints() {

	ttl := fingerprintTTL

	if lowEndDevice {
		ttl = ttl * 6
	}

	now := time.Now()
	var toRemove []string

	fingerprintsLock.RLock()
	for fingerprintHash, fp := range fingerprints {
		if now.Sub(fp.ReceiveTime) >= ttl {
			toRemove = append(toRemove, fingerprintHash)
		}
	}
	fingerprintsLock.RUnlock()

	fingerprintsLock.Lock()
	for _, fingerprintHash := range toRemove {
		delete(fingerprints, fingerprintHash)
	}
	fingerprintsLock.Unlock()
}

func getFingerprint(fingerprintHash []byte) *Fingerprint {
	fingerprintsLock.RLock()
	defer fingerprintsLock.RUnlock()
	fp, _ := fingerprints[string(fingerprintHash)]
	return fp
}

func addFingerprint(fingerprintHash []byte, receiveHash []byte) {
	fingerprintsLock.Lock()
	defer fingerprintsLock.Unlock()
	fingerprints[string(fingerprintHash)] = &Fingerprint{ReceiveTime: time.Now(), ReceiveHash: receiveHash}
}
