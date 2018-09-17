package snapshot

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"

	"../convert"
	"../db"
	"../db/ns"
	"../logs"
)

func LoadIRISnapshot(valuesPath string, spentPaths []string, timestamp int64) error {
	logs.Log.Noticef("Reading IRI snapshot, please do not kill the process or stop the computer: %v", valuesPath)
	if CurrentTimestamp > 0 {
		logs.Log.Info("It seems that the the tangle database already exists. Skipping snapshot load from file.")
		return nil
	}

	// Load values
	err := loadIRISnapshotValues(valuesPath)
	if err != nil {
		logs.Log.Error("Failed loading IRI snapshot values!", err)
		return err
	}
	// Load spent
	err = loadIRISnapshotSpents(spentPaths)
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

func loadIRISnapshotSpents(spentPaths []string) (err error) {
	for _, spentPath := range spentPaths {
		err = loadIRISnapshotSpent(spentPath)
		if err != nil {
			return err
		}
	}
	return err
}

func loadIRISnapshotSpent(spentPath string) error {
	logs.Log.Noticef("Loading spent addresses file: %v", spentPath)

	f, err := os.OpenFile(spentPath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	var dbTx = db.Singleton.NewTransaction(true)

	lineNr := 0
	for {
		lineNr++
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			logs.Log.Fatalf("read file line error (%d): %v", lineNr, err)
			return err
		}
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		if len(trimmedLine) != 81 {
			logs.Log.Fatalf("Wrong address in line (%d): %v", lineNr, trimmedLine)
		}

		err = loadSpentSnapshot(convert.TrytesToBytes(trimmedLine)[:49], dbTx)
		if err != nil {
			if err == db.ErrTransactionTooBig {
				err := dbTx.Commit()
				if err != nil {
					return err
				}
				dbTx = db.Singleton.NewTransaction(true)
				err = loadSpentSnapshot(convert.TrytesToBytes(trimmedLine)[:49], dbTx)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	return dbTx.Commit()
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
