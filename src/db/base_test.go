package db

import (
	"testing"
	"time"
	"reflect"
)

const hash = "HTULPSHIZIRNQMSEUNKFBQZRZ9JZVCIUZILZWWV9QVSNDRBLRHLYWTCPNFSJWBATJVSNMKUUFYSJA9999"
const hash2 = "HTZLPSHIZIRNQMSEUNKFBQZRZ9JZVCIUZILZWWV9QVSNDRBLRHLYWTCPNFSJWBATJVSNMKUUFYSJA9999"
const hash3 = "HTALPSHIZIRNQMSEUNKFBQZRZ9JZVCIUZILZWWV9QVSNDRBLRHLYWTCPNFSJWBATJVSNMKUUFYSJA9999"

func TestLoadTimedStore(t *testing.T) {
	store, err := LoadTimedStore("/tmp/timedstore.dat")
	if err != nil && store == nil {
		t.Error("Non-existing store load should return an empty one, but with an error")
	}
}

func TestLoadTimedStore2(t *testing.T) {
	var store LockableTimedHashStore
	store.Lock()
	store.Data = make(TimedHash)
	store.Data[hash] = time.Now()
	store.Data[hash2] = time.Now()
	store.Unlock()
	saved, err := store.save("/tmp/timedstore2.dat")
	if !saved || err != nil {
		t.Error("Store saving failed", err)
	}
	store2, err := LoadTimedStore("/tmp/timedstore2.dat")
	if err != nil {
		t.Error("Failed loading store", err)
	}
	theTime, ok := store2.Data[hash]
	if !ok {
		t.Error("Failed missing store value", hash)
	}
	if theTime.Unix() != store.Data[hash].Unix() {
		t.Error("Failed wrong store value", hash, theTime, store.Data[hash])
	}
	theTime, ok = store2.Data[hash2]
	if !ok {
		t.Error("Failed missing store value #2", hash2)
	}
	if theTime.Unix() != store.Data[hash2].Unix() {
		t.Error("Failed wrong store value #2", hash2, theTime, store.Data[hash2])
	}
}

func TestLoadTransactionStore(t *testing.T) {
	store, err := LoadTransactionStore("/tmp/tstore.dat")
	if err != nil && store == nil {
		t.Error("Non-existing store load should return an empty one, but with an error")
	}
}

func TestLoadTransactionStore2(t *testing.T) {
	var store TransactionStore
	store.Lock()
	store.Data = make(map[string]*DatabaseTransaction)
	store.Data[hash] = &DatabaseTransaction{hash2, hash3}
	store.Data[hash2] = &DatabaseTransaction{hash3, hash}
	store.Data[hash3] = &DatabaseTransaction{hash, hash2}
	store.Unlock()
	saved, err := store.save("/tmp/tstore2.dat")
	if !saved || err != nil {
		t.Error("Store saving failed", err)
	}
	store2, err := LoadTransactionStore("/tmp/tstore2.dat")
	if err != nil {
		t.Error("Failed loading store", err)
	}
	if !reflect.DeepEqual(store.Data[hash], store2.Data[hash]) {
		t.Error("Store value not found", hash)
	}
	if !reflect.DeepEqual(store.Data[hash2], store2.Data[hash2]) {
		t.Error("Store value not found", hash2)
	}
	if !reflect.DeepEqual(store.Data[hash3], store2.Data[hash3]) {
		t.Error("Store value not found", hash3)
	}
}