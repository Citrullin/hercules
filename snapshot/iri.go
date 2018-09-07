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
	"../db/ns"
	"../logs"
)

func LoadIRISnapshot(valuesPath string, spentPath string, timestamp int64) error {
	logs.Log.Notice("Reading IRI snapshot, please do not kill the process or stop the computer", valuesPath)
	if CurrentTimestamp > 0 {
		logs.Log.Warning("It seems that the the tangle database already exists. Skipping snapshot load from file.")
		return nil
	}
	db.Singleton.Lock()
	defer db.Singleton.Unlock()

	// Give time for other processes to finalize
	time.Sleep(WAIT_SNAPSHOT_DURATION)

	// Load values
	err := loadIRISnapshotValues(valuesPath)
	if err != nil {
		logs.Log.Error("Failed loading IRI snapshot values!", err)
		return err
	}
	// Load spent
	err = loadIRISnapshotSpent(spentPath)
	if err != nil {
		logs.Log.Error("Failed loading IRI snapshot spent flags!", err)
		return err
	}

	if checkDatabaseSnapshot() {
		SetSnapshotTimestamp(timestamp, nil)
		logs.Log.Notice("IRI Snapshot loaded")
	} else {
		logs.Log.Panic("Snapshot loading failed. The database is in an unstable state!")
	}
	return nil
}

func loadIRISnapshotSpent(spentPath string) error {
	f, err := os.OpenFile(spentPath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	var tx = db.Singleton.NewTransaction(true)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			logs.Log.Fatalf("read file line error: %v", err)
			return err
		}
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
	}

	return tx.Commit()
}

func loadIRISnapshotValues(valuesPath string) error {
	f, err := os.OpenFile(valuesPath, os.O_RDONLY, os.ModePerm)
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

	rd := bufio.NewReader(f)
	var total int64 = 0
	var txn = db.Singleton.NewTransaction(true)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			logs.Log.Fatalf("read file line error: %v", err)
			return err
		}
		tokens := strings.Split(line, ";")
		address := convert.TrytesToBytes(tokens[0])[:49]
		value, err := strconv.ParseInt(strings.TrimSpace(tokens[1]), 10, 64)
		if err != nil {
			return err
		}
		total += value
		err = loadValueSnapshot(address, value, txn)
		if err != nil {
			if err == db.ErrTransactionTooBig {
				err := txn.Commit()
				if err != nil {
					return err
				}
				txn = db.Singleton.NewTransaction(true)
				err = loadValueSnapshot(address, value, txn)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	logs.Log.Debugf("Snapshot total value: %v", total)

	return txn.Commit()
}
