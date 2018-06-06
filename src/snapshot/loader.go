package snapshot

import (
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"github.com/pkg/errors"
	"time"
	"bufio"
	"io"
	"strings"
	"strconv"
	"convert"
	"os"
)

func LoadSnapshot (path string) error {
	logs.Log.Info("Loading snapshot from", path)
	if CurrentTimestamp > 0 {
		logs.Log.Warning("It seems that the the tangle database already exists. Skipping snapshot load from file.")
		return nil
	}
	timestamp, err := checkSnapshotFile(path)
	if err != nil { return err }
	logs.Log.Debug("Timestamp:", timestamp)

	if !IsNewerThanSnapshot(int(timestamp), nil) {
		logs.Log.Infof("The given snapshot (%v) timestamp is older than the current one. Skipping", path)
		return nil
	}
	Lock(int(timestamp), path, nil)

	db.Locker.Lock()
	defer db.Locker.Unlock()

	// Give time for other processes to finalize
	time.Sleep(WAIT_SNAPSHOT_DURATION)

	logs.Log.Debug("Saving trimmable TXs flags...")
	err = trimData(timestamp)
	logs.Log.Debug("Saved trimmable TXs flags:", len(edgeTransactions))
	if err != nil { return err }

	err = doLoadSnapshot(path)
	if err != nil { return err }

	if checkDatabaseSnapshot() {
		return db.DB.Update(func(txn *badger.Txn) error {
			err:= SetSnapshotTimestamp(int(timestamp), txn)
			if err != nil { return err }

			err = Unlock(txn)
			if err != nil { return err }
			return nil
		})
	} else {
		return errors.New("failed database snapshot integrity check")
	}
}

func loadValueSnapshot(address []byte, value int64) error {
	addressKey := db.GetByteKey(address, db.KEY_SNAPSHOT_BALANCE)
	err := db.PutBytes(db.AsKey(addressKey, db.KEY_ADDRESS_BYTES), address, nil, nil)
	if err != nil { return err }
	err = db.Put(addressKey, value, nil, nil)
	if err != nil { return err }
	err = db.Put(db.AsKey(addressKey, db.KEY_BALANCE), value, nil, nil)
	if err != nil { return err }
	return nil
}

func loadSpentSnapshot(address []byte) error {
	addressKey := db.GetByteKey(address, db.KEY_SNAPSHOT_SPENT)
	err := db.PutBytes(db.AsKey(addressKey, db.KEY_ADDRESS_BYTES), address, nil, nil)
	if err != nil { return err }
	err = db.Put(addressKey, true, nil, nil)
	if err != nil { return err }
	err = db.Put(db.AsKey(addressKey, db.KEY_SPENT), true, nil, nil)
	if err != nil { return err }
	return nil
}

func loadPendingBundleSnapshot(key []byte) error {
	return db.Put(key, true, nil, nil)
}

func doLoadSnapshot (path string) error{
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	err = db.RemoveAll(db.KEY_BALANCE)
	err = db.RemoveAll(db.KEY_SNAPSHOT_BALANCE)
	if err != nil {
		return err
	}

	values := true
	keepBundles := false
	rd := bufio.NewReader(f)
	var total int64 = 0
	var totalSpent int64 = 0
	for {
		line, err := rd.ReadString('\n')
		line = strings.TrimSpace(line)
		if err != nil {
			if err == io.EOF {
				break
			}

			logs.Log.Fatalf("read file line error: %v", err)
			return err
		}
		if line == SNAPSHOT_SEPARATOR {
			if values {
				values = false
			} else {
				keepBundles = true
			}
			continue
		}
		if values {
			tokens := strings.Split(line, ";")
			address := convert.TrytesToBytes(tokens[0])[:49]
			value, err := strconv.ParseInt(tokens[1], 10, 64)
			if err != nil { return err }
			total += value
			err = loadValueSnapshot(address, value)
			if err != nil {
				return err
			}
		} else if !keepBundles {
			totalSpent++
			err = loadSpentSnapshot(convert.TrytesToBytes(strings.TrimSpace(line))[:49])
			if err != nil {
				return err
			}
		} else {
			key := convert.TrytesToBytes(line)[:16]
			err = loadPendingBundleSnapshot(key)
			if err != nil {
				return err
			}
		}
	}

	logs.Log.Debugf("Snapshot total value: %v", total)
	logs.Log.Debugf("Snapshot total spent addresses: %v", totalSpent)

	return nil
}