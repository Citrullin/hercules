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
	"../logs"
	"../db"
	"../convert"
	"../utils"
)

func SaveSnapshot (snapshotDir string, timestamp int) error {
	logs.Log.Noticef("Saving snapshot (%v) into %v...", timestamp, snapshotDir)
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

					line := convert.BytesToTrytes(key[1:])[:81] + ";" + strconv.FormatInt(int64(value), 10)
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
			line := convert.BytesToTrytes(key[1:])[:81]
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
