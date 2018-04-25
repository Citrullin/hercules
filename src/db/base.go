package db

import (
	"sync"
	"time"
	"os"
	"convert"
	"encoding/binary"
	"io"
	"path"
	"github.com/pkg/errors"
)

// TODO: hold account values in hash/int64

type TimedHash map[string]time.Time

type DatabaseTransaction struct {
	TrunkTransaction string
	BranchTransaction string
}

type TransactionDataStore struct {
	Data map[string]*string
	sync.RWMutex
}

type TransactionStore struct {
	Data map[string]*DatabaseTransaction
	databasePath string
	sync.RWMutex
}

type LockableTimedHashStore struct {
	Data TimedHash
	sync.RWMutex
}

func (store *TransactionStore) save (destination string) (bool, error) {
	store.RLock()
	// Hash: 49 bytes * 3
	f, err := os.Create(destination)
	defer f.Close()

	if err != nil {
		return false, err
	}

	data := make([]byte, 147)
	for key, value := range store.Data {
		copy(data[0:49], convert.TritsToBytes(convert.TrytesToTrits(key)))
		copy(data[49:98], convert.TritsToBytes(convert.TrytesToTrits(value.TrunkTransaction)))
		copy(data[98:147], convert.TritsToBytes(convert.TrytesToTrits(value.BranchTransaction)))
		f.Write(data)
	}
	f.Sync()
	store.RUnlock()
	return true, nil
}

func (store *LockableTimedHashStore) save (destination string) (bool, error) {
	store.RLock()
	// Hash: 49 bytes
	// Time: 8 bytes (seconds from unix time)
	f, err := os.Create(destination)
	defer f.Close()

	if err != nil {
		return false, err
	}

	data := make([]byte, 57)
	for key, value := range store.Data {
		timeData := make([]byte, 8)
		binary.LittleEndian.PutUint64(timeData, uint64(value.Unix()))
		copy(data[0:49], convert.TritsToBytes(convert.TrytesToTrits(key)))
		copy(data[49:57], timeData)
		f.Write(data)
	}
	f.Sync()
	store.RUnlock()
	return true, nil
}

func LoadTransactionStore (source string, databasePath string) (*TransactionStore, error) {
	var store TransactionStore
	store.databasePath = databasePath
	store.Data = make(map[string]*DatabaseTransaction)
	f, err := os.Open(source)
	defer f.Close()

	if err != nil {
		return &store, err
	}
	buffer := make([]byte, 147)
	for {
		_, err := f.Read(buffer)

		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}

		hash := convert.TritsToTrytes(convert.BytesToTrits(buffer[:49]))[:81]
		trunk := convert.TritsToTrytes(convert.BytesToTrits(buffer[49:98]))[:81]
		branch := convert.TritsToTrytes(convert.BytesToTrits(buffer[98:147]))[:81]
		store.Data[hash] = &DatabaseTransaction{trunk,branch}
	}
	return &store, nil
}

func LoadTimedStore (source string) (*LockableTimedHashStore, error) {
	var store LockableTimedHashStore
	store.Data = make(TimedHash)
	f, err := os.Open(source)
	defer f.Close()

	if err != nil {
		return &store, err
	}
	buffer := make([]byte, 57)
	for {
		_, err := f.Read(buffer)

		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}

		hash := convert.TritsToTrytes(convert.BytesToTrits(buffer[:49]))[:81]
		timeData := binary.LittleEndian.Uint64(buffer[49:57])
		store.Data[hash] = time.Unix(int64(timeData), 0)
	}
	return &store, nil
}

func (store *LockableTimedHashStore) Has (hash string) bool {
	store.RLock()
	_, ok := store.Data[hash]
	store.RUnlock()
	return ok
}

func (store *TransactionStore) Has (hash string) bool {
	store.RLock()
	_, ok := store.Data[hash]
	store.RUnlock()
	return ok
}

func (store *LockableTimedHashStore) Get (hash string) (time.Time, bool) {
	store.RLock()
	t, ok := store.Data[hash]
	store.RUnlock()
	return t, ok
}

func (store *TransactionStore) Get (hash string) (*DatabaseTransaction, bool) {
	store.RLock()
	t, ok := store.Data[hash]
	store.RUnlock()
	return t, ok
}

func (store *TransactionStore) GetTrytes (hash string) (string, error) {
	path := store.getObjectPath(hash)
	f, err := os.Open(path)
	defer f.Close()

	if err != nil {
		return "", err
	}

	buffer := make([]byte, 1604)
	count, err := f.Read(buffer)
	if err != nil {
		return "", err
	}

	// Corrupted file, remove
	if count < 1604 {
		os.Remove(path)
		return "", errors.New("Corrupted TX object detected")
	}

	return convert.TritsToTrytes(convert.BytesToTrits(buffer))[:2673], nil
}

func (store *LockableTimedHashStore) Count () int {
	store.RLock()
	l := len(store.Data)
	store.RUnlock()
	return l
}

func (store *TransactionStore) Count () int {
	store.RLock()
	l := len(store.Data)
	store.RUnlock()
	return l
}

func (store *LockableTimedHashStore) Save (hash string, t time.Time) {
	store.Lock()
	store.Data[hash] = t
	store.Unlock()
}

func (store *LockableTimedHashStore) SaveNow (hash string) {
	store.Lock()
	store.Data[hash] = time.Now()
	store.Unlock()
}

func (store *TransactionStore) Save (hash string, data *DatabaseTransaction, trytes *string) {
	store.Lock()
	store.Data[hash] = data
	store.Unlock()
	go store.saveTrytes(hash, trytes)
}

func (store *LockableTimedHashStore) Delete (hash string) {
	defer func () { recover() }()
	store.Lock()
	_, ok := store.Data[hash]
	if ok {
		delete(store.Data, hash)
	}
	store.Unlock()
}

func (store *TransactionStore) Delete (hash string) {
	defer func () { recover() }()
	store.Lock()
	delete(store.Data, hash)
	store.Unlock()
	go store.deleteTrytes(hash)
}

func (store *TransactionStore) deleteTrytes(hash string) {
	path := store.getObjectPath(hash)
	os.Remove(path)
}

func (store *TransactionStore) saveTrytes (hash string, trytes *string) (bool, error) {
	path := store.getObjectPath(hash)
	if !exists(path) {
		f, err := os.Create(path)
		defer f.Close()

		if err != nil {
			return false, err
		}

		f.Write(convert.TritsToBytes(convert.TrytesToTrits(*trytes)))
		f.Sync()
		return true, nil
	}
	return false, nil
}

func (store *TransactionStore) getObjectPath (hash string) string {
	return path.Join(store.databasePath, hash)
}

func exists (path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}