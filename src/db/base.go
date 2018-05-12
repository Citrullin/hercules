package db

import (
	"github.com/dgraph-io/badger"
	"crypto/md5"
	"encoding/gob"
	"bytes"
	"time"
	"utils"
)

const (
	KEY_FINGERPRINT = byte(0) // hash -> tx.hash trytes

	// TRANSACTION SAVING
	KEY_HASH      = byte(1) // hash -> tx.hash
	KEY_TIMESTAMP = byte(2) // hash -> time
	KEY_BYTES     = byte(3) // hash -> raw tx trytes
	KEY_BUNDLE    = byte(4) // bundle hash + tx hash -> index
	KEY_ADDRESS   = byte(5) // address hash + hash -> value
	KEY_TAG       = byte(6) // tag hash + hash -> empty
	KEY_VALUE     = byte(7) // hash -> int64
	KEY_ADDRESS_HASH   = byte(8) // hash -> address

	KEY_EDGE      = byte(10) // hash -> time

	// RELATIONS
	KEY_RELATION = byte(15) // hash -> hash+hash
	KEY_APPROVEE = byte(16) // hash + parent hash -> empty

	// MILESTONE/CONFIRMATION RELATED
	KEY_MILESTONE       = byte(20) // hash -> index
	KEY_SOLID_MILESTONE = byte(21) // hash -> index
	KEY_CONFIRMED       = byte(25) // hash -> time
	KEY_TIP             = byte(27) // hash -> time

	// PENDING + UNKNOWN CONFIRMED TRANSACTIONS
	KEY_PENDING           = byte(30) // hash -> parent time
	KEY_PENDING_HASH      = byte(31) // hash -> hash
	KEY_PENDING_CONFIRMED = byte(35) // hash -> parent time

	// PERSISTENT EVENTS
	KEY_EVENT_MILESTONE_PENDING        = byte(50)  // trunk hash (999 address) -> tx hash
	KEY_EVENT_MILESTONE_PAIR_PENDING   = byte(51)  // trunk hash (999 address) -> tx hash
	KEY_EVENT_CONFIRMATION_PENDING     = byte(56)  // hash (coo address) -> index

	// OTHER
	KEY_BALANCE                   = byte(100) // address hash -> int64
	KEY_SPENT                     = byte(101) // address hash -> bool
	KEY_SNAPSHOT                  = byte(120) // hash -> hash+int64+hash+int64+...
	KEY_TEST                      = byte(187) // hash -> bool
	KEY_OTHER                     = byte(255) // XXXX -> any bytes
)

type KeyValue struct {
	Key []byte
	Value []byte
}

// Returns a 16-bytes key based on other key
func AsKey(keyBytes []byte, key byte) []byte {
	b := make([]byte, 16)
	copy(b, keyBytes)
	b[0] = key
	return b
}

// Returns a 16-bytes key based on bytes
func GetByteKey(bytes []byte, key byte) []byte {
	b := md5.Sum(bytes)
	b[0] = key
	return b[:]
}

func Has(key []byte, txn *badger.Txn) bool {
	tx := txn
	var err error = nil
	if txn == nil {
		tx = DB.NewTransaction(false)
		defer func() error {
			tx.Commit(func(e error) {})
			return err
		}()
	}
	_, err = tx.Get(key)
	return err == nil
}

func Put(key []byte, value interface{}, ttl *time.Duration, txn *badger.Txn) error {
	tx := txn
	var err error = nil
	if txn == nil {
		tx = DB.NewTransaction(true)
		defer func() error {
			if err != nil {
				tx.Discard()
				return err
			}
			return tx.Commit(func(e error) {})
		}()
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err = enc.Encode(value)
	if err != nil {
		return err
	}
	if ttl != nil {
		return tx.SetWithTTL(key, buf.Bytes(), *ttl)
	}
	return tx.Set(key, buf.Bytes())
}

func PutBytes(key []byte, value []byte, ttl *time.Duration, txn *badger.Txn) error {
	tx := txn
	var err error = nil
	if txn == nil {
		tx = DB.NewTransaction(true)
		defer func() error {
			if err != nil {
				tx.Discard()
				return err
			}
			return tx.Commit(func(e error) {})
		}()
	}
	if ttl != nil {
		err = tx.SetWithTTL(key, value, *ttl)
	} else {
		err = tx.Set(key, value)
	}
	return err
}

func Get(key []byte, data interface{}, txn *badger.Txn) error {
	tx := txn
	var err error = nil
	if txn == nil {
		tx = DB.NewTransaction(false)
		defer func() error {
			if err != nil {
				tx.Discard()
				return err
			}
			return tx.Commit(func(e error) {})
		}()
	}
	item, err := tx.Get(key)
	if err != nil {
		return err
	}
	val, err := item.Value()
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(val)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(data)
	if err != nil {
		return err
	}
	return nil
}

func GetBytes(key []byte, txn *badger.Txn) ([]byte, error) {
	var resp []byte = nil
	err := Get(key, &resp, txn)
	return resp, err
}

func GetInt(key []byte, txn *badger.Txn) (int, error) {
	var resp = 0
	err := Get(key, &resp, txn)
	return resp, err
}

func GetBool(key []byte, txn *badger.Txn) (bool, error) {
	var resp = false
	err := Get(key, &resp, txn)
	return resp, err
}

func GetInt64(key []byte, txn *badger.Txn) (int64, error) {
	var resp int64 = 0
	err := Get(key, &resp, txn)
	return resp, err
}

func Remove(key []byte, txn *badger.Txn) error {
	tx := txn
	if txn == nil {
		tx = DB.NewTransaction(true)
		defer tx.Commit(func(e error) {})
	}
	return tx.Delete(key)
}

func Count(key byte) int {
	count := 0
	_ = DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{key}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	})
	return count
}

func CountByPrefix(prefix []byte) int {
	count := 0
	_ = DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	})
	return count
}

/*
Returns latest key iterating over all items of certain type.
The value is expected to be a unix timestamp
 */
func GetLatestKey(key byte, txn *badger.Txn) ([]byte, int, error) {
	tx := txn
	if txn == nil {
		tx = DB.NewTransaction(false)
		defer tx.Commit(func(e error) {})
	}
	var latest []byte
	var current = 0
	it := tx.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	prefix := []byte{key}
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		v, err := item.Value()
		if err != nil {
			return nil, 0, err
		}
		var data int
		buf := bytes.NewBuffer(v)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(data)
		if err != nil {
			return nil, 0, err
		}
		if data > current {
			current = data
			latest = item.Key()
		}
	}
	return latest, current, nil
}

/*
Returns latest random key iterating over all items of certain type.
The value is expected to be a unix timestamp. //When picked, the value is updated
 */
func PickRandomKey(key byte, txn *badger.Txn) []byte {
	tx := txn
	if txn == nil {
		tx = DB.NewTransaction(false)
		defer tx.Commit(func(e error) {})
	}
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	it := tx.NewIterator(opts)
	max := Count(key)
	if max == 0 { return nil }
	target := utils.Random(0, max)
	step := 0
	defer it.Close()

	// TODO: (OPT) when some buffers (like requests) fill up to a certain point, remove half of them, maybe?
	var result []byte = nil

	prefix := []byte{key}
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		if step == target {
			return it.Item().Key()
		}
		step++
	}
	return result
}

func IncrBy(key []byte, value int64, deleteOnZero bool, txn *badger.Txn) (int64, error) {
	balance, err := GetInt64(key, txn)
	balance += value
	if balance == 0 && deleteOnZero && err != nil {
		Remove(key, txn)
	}
	err = Put(key, balance, nil, txn)
	return balance, err
}
