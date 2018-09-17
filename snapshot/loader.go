package snapshot

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"github.com/pkg/errors"
)

func LoadSnapshot(path string) error {
	logs.Log.Info("Loading snapshot from", path)
	if CurrentTimestamp > 0 {
		logs.Log.Info("It seems that the the tangle database already exists. Skipping snapshot load from file.")
		return nil
	}
	timestamp, err := checkSnapshotFile(path)
	if err != nil {
		return err
	}
	logs.Log.Debug("Timestamp:", timestamp)

	if !IsNewerThanSnapshot(timestamp, nil) {
		logs.Log.Infof("The given snapshot (%v) timestamp is older than the current one. Skipping", path)
		return nil
	}
	Lock(timestamp, path, nil)

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
		return db.Singleton.Update(func(dbTx db.Transaction) error {
			err := SetSnapshotTimestamp(timestamp, dbTx)
			if err != nil {
				return err
			}

			err = Unlock(dbTx)
			if err != nil {
				return err
			}
			return nil
		})
	} else {
		return errors.New("failed database snapshot integrity check")
	}
}

func loadValueSnapshot(address []byte, value int64, dbTx db.Transaction) error {
	addressKey := ns.AddressKey(address, ns.NamespaceSnapshotBalance)
	err := coding.PutInt64(dbTx, addressKey, value)
	if err != nil {
		return err
	}
	err = coding.PutInt64(dbTx, ns.AddressKey(address, ns.NamespaceBalance), value)
	if err != nil {
		return err
	}
	return nil
}

func loadSpentSnapshot(address []byte, dbTx db.Transaction) error {
	// err := dbTx.PutBytes(ns.AddressKey(address, ns.NamespaceSnapshotSpent), address)
	// if err != nil {
	// 	return err
	// }
	err := coding.PutBool(dbTx, ns.AddressKey(address, ns.NamespaceSnapshotSpent), true)
	if err != nil {
		return err
	}
	err = coding.PutBool(dbTx, ns.AddressKey(address, ns.NamespaceSpent), true)
	if err != nil {
		return err
	}
	return nil
}

func loadKey(key []byte, timestamp int64) error {
	return coding.PutInt64(db.Singleton, key, timestamp)
}

func doLoadSnapshot(path string, timestamp int64) error {
	logs.Log.Infof("Loading values from %v. It can take several minutes. Please hold...", path)
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	err = ns.Remove(db.Singleton, ns.NamespaceBalance)
	if err != nil {
		return err
	}
	err = ns.Remove(db.Singleton, ns.NamespaceSnapshotBalance)
	if err != nil {
		return err
	}

	stage := 0
	rd := bufio.NewReader(f)
	var total int64 = 0
	var totalSpent int64 = 0
	var firstLine = true
	var dbTx = db.Singleton.NewTransaction(true)

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
			err = loadValueSnapshot(address, value, dbTx)
			if err != nil {
				if err == db.ErrTransactionTooBig {
					err := dbTx.Commit()
					if err != nil {
						return err
					}
					dbTx = db.Singleton.NewTransaction(true)
					err = loadValueSnapshot(address, value, dbTx)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			}
		} else if stage == 1 {
			totalSpent++
			err = loadSpentSnapshot(convert.TrytesToBytes(strings.TrimSpace(line))[:49], dbTx)
			if err != nil {
				if err == db.ErrTransactionTooBig {
					err := dbTx.Commit()
					if err != nil {
						return err
					}
					dbTx = db.Singleton.NewTransaction(true)
					err = loadSpentSnapshot(convert.TrytesToBytes(strings.TrimSpace(line))[:49], dbTx)
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
					err := dbTx.Commit()
					if err != nil {
						return err
					}
					dbTx = db.Singleton.NewTransaction(true)
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

	err = dbTx.Commit()
	if err != nil {
		return err
	}

	logs.Log.Debugf("Snapshot total value: %v", total)
	logs.Log.Debugf("Snapshot total spent addresses: %v", totalSpent)

	return nil
}
