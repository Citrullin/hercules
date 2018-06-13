package snapshot

import (
	"path"
	"os"
	"bufio"
	"bytes"
	"strconv"
	"encoding/gob"
	"fmt"
	"sort"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules/logs"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/utils"
	"gitlab.com/semkodev/hercules/transaction"
)

func SaveSnapshot (snapshotDir string, timestamp int) error {
	logs.Log.Noticef("Saving snapshot (%v) into %v...", timestamp, snapshotDir)
	db.Locker.Lock()
	defer db.Locker.Unlock()
	utils.CreateDirectory(snapshotDir)

	savepth := path.Join(snapshotDir, strconv.FormatInt(int64(timestamp), 10) + ".snap")
	pth := savepth + "_"
	file, err := os.Create(pth)
	if err != nil {
		logs.Log.Noticef("Could not create snapshot file: %v", pth)
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)

	var lineBuffer []string

	var addToBuffer = func (line string) {
		if lowEndDevice {
			fmt.Fprintln(w, line)
		} else {
			lineBuffer = append(lineBuffer, line)
		}
	}

	var commitBuffer = func () {
		defer func() { lineBuffer = nil } ()
		if lowEndDevice || lineBuffer == nil { return }
		sort.Strings(lineBuffer)
		for _, line := range lineBuffer {
			fmt.Fprintln(w, line)
		}
	}

	err = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_SNAPSHOT_BALANCE}

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
						addressHash = restoreBrokenAddress(db.AsKey(key, db.KEY_ADDRESS_BYTES))
					}
					line := convert.BytesToTrytes(addressHash)[:81] + ";" + strconv.FormatInt(int64(value), 10)
					addToBuffer(line)
				} else {
					logs.Log.Error("Could not parse a snapshot value from database!", err)
					return err
				}
			} else {
				logs.Log.Error("Could not read a snapshot value from database!", err)
				return err
			}
		}
		commitBuffer()
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
			addToBuffer(line)
		}
		commitBuffer()
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
				logs.Log.Error("Could not get keep Bundle from the database!", err)
				return err
			}
			line := convert.BytesToTrytes(key)
			addToBuffer(line)
		}
		commitBuffer()
		return nil
	})
	if err != nil { return err }


	fmt.Fprintln(w, SNAPSHOT_SEPARATOR)
	err = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_SNAPSHOTTED}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			if err != nil {
				logs.Log.Error("Could not get ignore TX from the database!", err)
				return err
			}
			line := convert.BytesToTrytes(key)
			addToBuffer(line)
		}
		commitBuffer()
		return nil
	})
	if err != nil { return err }
	logs.Log.Notice("Snapshot saved, flushing...")
	err = w.Flush()
	if err != nil { return err }
	return os.Rename(pth, savepth)
}

func restoreBrokenAddress (key []byte) []byte {
	logs.Log.Warningf("Address hash not found for key %v. Trying to restore...", key)
	var hash []byte
	db.DB.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := db.AsKey(key, db.KEY_ADDRESS)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			txHashKey := it.Item().Key()[16:]
			txBytes, err := db.GetBytes(db.AsKey(txHashKey, db.KEY_BYTES), txn)
			if err != nil { continue }
			trits := convert.BytesToTrits(txBytes)[:8019]
			tx := transaction.TritsToFastTX(&trits, txBytes)
			hash = tx.Address
			break
		}
		if hash != nil {
			logs.Log.Warningf(" Found address: %v. Restoring...", convert.BytesToTrytes(hash)[:81])
			return db.PutBytes(db.GetByteKey(hash, db.KEY_ADDRESS_BYTES), hash, nil, txn)
		} else {
			logs.Log.Warning("Address not found. Trying to find in transactions data")
			x := 0
			opts := badger.DefaultIteratorOptions
			it := txn.NewIterator(opts)
			defer it.Close()
			prefix := []byte{db.KEY_BYTES}
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				x++
				if x % 100000 == 0 {
					logs.Log.Debugf("Progress %v", x)
				}
				txHashKey := it.Item().Key()[16:]
				txBytes, err := db.GetBytes(db.AsKey(txHashKey, db.KEY_BYTES), txn)
				if err != nil { continue }
				trits := convert.BytesToTrits(txBytes)[:8019]
				tx := transaction.TritsToFastTX(&trits, txBytes)
				if bytes.Equal(key, db.GetByteKey(tx.Address, db.KEY_ADDRESS_BYTES)) {
					hash = tx.Address
					break
				}
			}
		}
		if hash != nil {
			logs.Log.Warningf(" Found address: %v. Restoring...", convert.BytesToTrytes(hash)[:81])
			return db.PutBytes(db.GetByteKey(hash, db.KEY_ADDRESS_BYTES), hash, nil, txn)
		}
		return nil
	})
	return hash
}
