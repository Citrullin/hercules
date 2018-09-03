package snapshot

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"../convert"
	"../db"
	"../logs"
	"github.com/pkg/errors"
)

func LoadSnapshot(path string) error {
	logs.Log.Info("Loading snapshot from", path)
	if CurrentTimestamp > 0 {
		logs.Log.Warning("It seems that the the tangle database already exists. Skipping snapshot load from file.")
		return nil
	}
	timestamp, err := checkSnapshotFile(path)
	if err != nil {
		return err
	}
	logs.Log.Debug("Timestamp:", timestamp)

	if !IsNewerThanSnapshot(int(timestamp), nil) {
		logs.Log.Infof("The given snapshot (%v) timestamp is older than the current one. Skipping", path)
		return nil
	}
	Lock(int(timestamp), path, nil)

	db.Singleton.Lock()
	defer db.Singleton.Unlock()

	// Give time for other processes to finalize
	time.Sleep(WAIT_SNAPSHOT_DURATION)

	logs.Log.Debug("Saving trimmable TXs flags...")
	err = trimData(timestamp)
	logs.Log.Debug("Saved trimmable TXs flags:", len(edgeTransactions))
	if err != nil {
		return err
	}

	err = doLoadSnapshot(path, timestamp)
	if err != nil {
		return err
	}

	if checkDatabaseSnapshot() {
		return db.Singleton.Update(func(tx db.Transaction) error {
			err := SetSnapshotTimestamp(int(timestamp), tx)
			if err != nil {
				return err
			}

			err = Unlock(tx)
			if err != nil {
				return err
			}
			return nil
		})
	} else {
		return errors.New("failed database snapshot integrity check")
	}
}

func loadValueSnapshot(address []byte, value int64, tx db.Transaction) error {
	addressKey := db.GetAddressKey(address, db.KEY_SNAPSHOT_BALANCE)
	err := tx.Put(addressKey, value, nil)
	if err != nil {
		return err
	}
	err = tx.Put(db.GetAddressKey(address, db.KEY_BALANCE), value, nil)
	if err != nil {
		return err
	}
	return nil
}

func loadSpentSnapshot(address []byte, tx db.Transaction) error {
	err := tx.PutBytes(db.GetAddressKey(address, db.KEY_SNAPSHOT_SPENT), address, nil)
	if err != nil {
		return err
	}
	err = tx.Put(db.GetAddressKey(address, db.KEY_SNAPSHOT_SPENT), true, nil)
	if err != nil {
		return err
	}
	err = tx.Put(db.GetAddressKey(address, db.KEY_SPENT), true, nil)
	if err != nil {
		return err
	}
	return nil
}

func loadKey(key []byte, timestamp int64) error {
	return db.Singleton.Put(key, timestamp, nil)
}

func doLoadSnapshot(path string, timestamp int64) error {
	logs.Log.Infof("Loading values from %v. It can take several minutes. Please hold...", path)
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	err = db.Singleton.RemoveKeyCategory(db.KEY_BALANCE)
	err = db.Singleton.RemoveKeyCategory(db.KEY_SNAPSHOT_BALANCE)
	if err != nil {
		return err
	}

	stage := 0
	rd := bufio.NewReader(f)
	var total int64 = 0
	var totalSpent int64 = 0
	var firstLine = true
	var tx = db.Singleton.NewTransaction(true)

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
			stage++
			continue
		}

		if firstLine {
			firstLine = false
			tokens := strings.Split(line, ";")
			if len(tokens) < 2 {
				// Header
				continue
			}
		}
		if stage == 0 {
			tokens := strings.Split(line, ";")
			address := convert.TrytesToBytes(tokens[0])[:49]
			value, err := strconv.ParseInt(tokens[1], 10, 64)
			if err != nil {
				return err
			}
			total += value
			err = loadValueSnapshot(address, value, tx)
			if err != nil {
				if err == db.ErrTransactionTooBig {
					err := tx.Commit()
					if err != nil {
						return err
					}
					tx = db.Singleton.NewTransaction(true)
					err = loadValueSnapshot(address, value, tx)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
		} else if stage == 1 {
			totalSpent++
			err = loadSpentSnapshot(convert.TrytesToBytes(strings.TrimSpace(line))[:49], tx)
			if err != nil {
				if err == db.ErrTransactionTooBig {
					err := tx.Commit()
					if err != nil {
						return err
					}
					tx = db.Singleton.NewTransaction(true)
					err = loadSpentSnapshot(convert.TrytesToBytes(strings.TrimSpace(line))[:49], tx)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
		} else {
			key := convert.TrytesToBytes(line)[:16]
			err = loadKey(key, timestamp)
			if err != nil {
				if err == db.ErrTransactionTooBig {
					err := tx.Commit()
					if err != nil {
						return err
					}
					tx = db.Singleton.NewTransaction(true)
					err = loadKey(key, timestamp)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	logs.Log.Debugf("Snapshot total value: %v", total)
	logs.Log.Debugf("Snapshot total spent addresses: %v", totalSpent)

	return nil
}
