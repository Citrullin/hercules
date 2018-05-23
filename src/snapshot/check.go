package snapshot

import (
	"logs"
	"db"
	"github.com/dgraph-io/badger"
	"bytes"
	"encoding/gob"
	"strings"
	"path/filepath"
	"strconv"
	"time"
	"github.com/pkg/errors"
	"bufio"
	"io"
	"os"
)

func checkDatabaseSnapshot () bool {
	logs.Log.Info("Checking database snapshot integrity")
	var total int64 = 0

	err := db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_SNAPSHOT_BALANCE}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			v, err := it.Item().Value()
			if err == nil {
				var value int64 = 0
				buf := bytes.NewBuffer(v)
				dec := gob.NewDecoder(buf)
				err := dec.Decode(&value)
				if err == nil {
					total += value
				} else {
					logs.Log.Error("Could not parse a snapshot value from database!")
					return err
				}
			} else {
				logs.Log.Error("Could not read a snapshot value from database!")
				return err
			}
		}
		return nil
	})
	if err != nil { return false }
	if total == TOTAL_IOTAS {
		logs.Log.Info("Database snapshot integrity check passed")
		return true
	} else {
		logs.Log.Info("Database snapshot integrity check failed")
		return false
	}
}

func checkSnapshotFile (path string) (timestamp int64, err error) {
	// Check timestamp
	filename := filepath.Base(path)
	timestamp, err = strconv.ParseInt(strings.Split(filename, ".")[0], 10, 32)

	if err != nil || timestamp < TIMESTAMP_MIN || timestamp > time.Now().Unix() {
		return 0, errors.New("timestamp validation failed")
	}

	current, err := db.GetInt([]byte{db.KEY_SNAPSHOT_DATE}, nil)
	if err == nil && int64(current) > timestamp {
		logs.Log.Errorf(
			"The current snapshot (%v) is more recent than the one being loaded (%v)!",
			time.Unix(int64(current), 0),
			time.Unix(timestamp, 0))
		return 0, errors.New("current snapshot more recent")
	}

	err = checkSnapshotFileIntegrity(path)
	if err != nil { return 0, err }

	return timestamp, nil
}

func checkSnapshotFileIntegrity (path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	var checkingSpent = false
	var total int64 = 0
	var totalSpent int64 = 0

	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadString('\n')
		line = strings.TrimSpace(line)
		if err != nil {
			if err == io.EOF {
				break
			}

			logs.Log.Errorf("Read file line error: %v", err)
			return err
		}
		if line == SNAPSHOT_SEPARATOR {
			checkingSpent = true
		} else {
			if checkingSpent {
				totalSpent++
			} else {
				tokens := strings.Split(line, ";")
				value, err := strconv.ParseInt(tokens[1], 10, 64)
				if err != nil {
					logs.Log.Errorf("Error parsing address value: %v => %v", tokens[1], err)
					return err
				}
				total += value
			}
		}
	}

	if totalSpent < MIN_SPENT_ADDRESSES {
		logs.Log.Error("Spent addresses count is wrong!")
		return errors.New("spent addresses validation failed")
	}

	if total != TOTAL_IOTAS {
		logs.Log.Errorf("Address balances validation failed %v vs %v!", TOTAL_IOTAS, total)
		return errors.New("address balance validation failed")
	}

	return nil
}
