package snapshot

import (
	"logs"
	"db"
	"path"
	"os"
	"bufio"
	"github.com/dgraph-io/badger"
	"bytes"
	"strconv"
	"encoding/gob"
	"convert"
	"fmt"
	"utils"
)

func SaveSnapshot (snapshotDir string, timestamp int) error {
	logs.Log.Noticef("Saving snapshot (%v) into %v...", timestamp, snapshotDir)
	db.Locker.Lock()
	defer db.Locker.Unlock()
	utils.CreateDirectory(snapshotDir)

	pth := path.Join(snapshotDir, strconv.FormatInt(int64(timestamp), 10) + ".snap")
	file, err := os.Create(pth)
	if err != nil {
		logs.Log.Noticef("Could not create snapshot file: %v", pth)
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)

	err = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_SNAPSHOT_BALANCE}
		// TODO: order by address
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			v, err := item.Value()
			if err == nil {
				var value int64 = 0
				buf := bytes.NewBuffer(v)
				dec := gob.NewDecoder(buf)
				err := dec.Decode(&value)
				if err == nil {
					// Do not save zero-value addresses
					if value == 0 { continue }

					item, err := txn.Get(db.AsKey(key, db.KEY_ADDRESS_BYTES))
					if err != nil {
						logs.Log.Error("Could not get an address hash value from database!", err, key)
						return err
					}
					addressHash, err := item.Value()
					if err != nil {
						logs.Log.Error("Could not parse an address hash value from database!", err)
						return err
					}
					if len(addressHash) < 49 {
						logs.Log.Errorf("Wrong address length! value %v, (%v) => %v, key: %v", value, convert.BytesToTrytes(addressHash), addressHash, key)
					}
					line := convert.BytesToTrytes(addressHash)[:81] + ";" + strconv.FormatInt(int64(value), 10)
					fmt.Fprintln(w, line)
				} else {
					logs.Log.Error("Could not parse a snapshot value from database!", err)
					return err
				}
			} else {
				logs.Log.Error("Could not read a snapshot value from database!", err)
				return err
			}
		}
		return nil
	})

	fmt.Fprintln(w, SNAPSHOT_SEPARATOR)
	err = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_SNAPSHOT_SPENT}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			item, err := txn.Get(db.AsKey(key, db.KEY_ADDRESS_BYTES))
			if err != nil {
				logs.Log.Error("Could not get an address hash spent from database!", err)
				return err
			}
			addressHash, err := item.Value()
			if err != nil {
				logs.Log.Error("Could not parse an address hash spent from database!", err)
				return err
			}
			line := convert.BytesToTrytes(addressHash)[:81]
			fmt.Fprintln(w, line)
		}
		return nil
	})
	if err != nil { return err }


	fmt.Fprintln(w, SNAPSHOT_SEPARATOR)
	err = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_PENDING_BUNDLE}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			if err != nil {
				logs.Log.Error("Could not get ignore TX timestamp from the database!", err)
				return err
			}
			line := convert.BytesToTrytes(key)
			fmt.Fprintln(w, line)
		}
		return nil
	})
	if err != nil { return err }
	logs.Log.Notice("Snapshot saved, flushing...")
	return w.Flush()
}
