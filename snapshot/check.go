package snapshot

import (
	"bytes"
	"encoding/gob"
	"strings"
	"path/filepath"
	"strconv"
	"time"
	"bufio"
	"io"
	"os"

	"../logs"
	"../db"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
)

/*
Returns if the given timestamp is more recent than the current database snapshot.
 */
func IsNewerThanSnapshot(timestamp int, txn *badger.Txn) bool {
	current := GetSnapshotTimestamp(txn)
	return timestamp > current
}

/*
Returns if the given timestamp is more recent than the current database snapshot.
 */
func IsEqualOrNewerThanSnapshot(timestamp int, txn *badger.Txn) bool {
	current := GetSnapshotTimestamp(txn)
	return timestamp >= current
}

/*
Returns whether the current tangle is synchronized
 */
 // TODO: this check is too slow on bigger databases. The counters should be moved to memory.
func IsSynchronized () bool {
	return db.Count(db.KEY_PENDING_CONFIRMED) < 10 &&
		db.Count(db.KEY_EVENT_CONFIRMATION_PENDING) < 10 &&
		db.Count(db.KEY_EVENT_MILESTONE_PENDING) < 5
}

/*
Checks outstanding pending confirmations that node is beyond the snapshot horizon.
This is just an additional measure to prevent tangle inconsistencies.
 */
func CanSnapshot(timestamp int) bool {
	pendingConfirmationsBehindHorizon := false
	err := db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_EVENT_CONFIRMATION_PENDING}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			v, err := it.Item().Value()
			if err != nil {
				return err
			}
			var ts = 0
			buf := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buf)
			err = dec.Decode(&ts)
			if err != nil {
				return err
			}
			if ts > 0 && ts <= timestamp {
				pendingConfirmationsBehindHorizon = true
				break
			}
		}
		return nil
	})
	return err == nil && !pendingConfirmationsBehindHorizon
}

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
		logs.Log.Errorf("Database snapshot integrity check failed: %v should be %v", total, TOTAL_IOTAS)
		logs.Log.Fatal("The database is in an inconsistent state now :(. Dying...")
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

/*
Checks if there is a snapshot lock present.
Yes:
If lock file is present, run LoadSnapshot.
Otherwise run MakeSnapshot.
 */
func checkPendingSnapshot () {
	timestamp, filename := IsLocked(nil)
	if timestamp >= 0 {
		if len(filename) > 0 {
			newFilename := config.GetString("snapshots.loadFile")
			if len(newFilename) > 0 {
				filename = newFilename
			}
			logs.Log.Info("Found pending snapshot lock. Trying to continue... ", filename)
			LoadSnapshot(filename)
		} else {
			logs.Log.Info("Found pending snapshot lock. Trying to continue... ", timestamp)
			MakeSnapshot(timestamp)
		}
	}
}
