package db

import (
	"github.com/dgraph-io/badger"
	"log"
	"convert"
	"crypto/md5"
	"encoding/gob"
	"bytes"
)

// TODO: write tests
var DB *badger.DB

const (
	KEY_HASH = byte(1) // hash -> tx.hash trytes
	KEY_TIMESTAMP = byte(2) // hash -> time
	KEY_TRANSACTION = byte(3) // hash -> raw tx trytes
	KEY_RELATION = byte(4) // hash -> hash+hash
	KEY_CONFIRMED = byte(5) // hash -> time
	KEY_MILESTONE = byte(6) // hash -> time
	KEY_REQUESTS = byte(20) // hash -> time
	KEY_UNKNOWN = byte(21) // hash -> time
	KEY_ACCOUNT = byte(100) // hash -> int64
	KEY_SNAPSHOT = byte(120) // hash -> hash+int64+hash+int64+...
)

func LoadDB (config *DatabaseConfig) {
	opts := badger.DefaultOptions
	opts.Dir = config.Path
	opts.ValueDir = config.Path
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	DB = db
	db.PurgeOlderVersions()
	db.RunValueLogGC(0.5)
}

func EndDB () {
	DB.Close()
}

// Returns a 16-bytes key based on a string hash
func GetHashKey (hash string, key byte) []byte {
	resp := md5.Sum(convert.TrytesToBytes(hash))
	resp[0] = key
	return resp[:]
}

// Returns a 16-bytes key based on bytes
func GetByteKey (bytes []byte, key byte) []byte {
	b := md5.Sum(bytes)
	b[0] = key
	return b[:]
}

func Has (key []byte, txn *badger.Txn) bool {
	_, err := txn.Get(key)
	return !(err != nil)
}

func Put (key []byte, value interface{}, txn *badger.Txn) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(value)
	if err != nil {
		return err
	}
	return txn.Set(key, buf.Bytes())
}

func Get (key []byte, data interface{}, txn *badger.Txn) error {
	item, err := txn.Get(key)
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

func Remove (key []byte, txn *badger.Txn) error {
	return txn.Delete(key)
}

func Count(key byte) int {
	count := 0
	_ = DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{key}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next(){
			count++
		}
		return nil
	})
	return count
}

type Hashable struct {
	key byte
}

type IHashable interface {
	Get()
}
