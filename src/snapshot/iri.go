package snapshot

import (
	"logs"
	"db"
	"time"
	"os"
	"bufio"
	"io"
	"convert"
	"strings"
	"strconv"
)

func LoadIRISnapshot(valuesPath string, spentPath string, timestamp int) error {
	logs.Log.Notice("Reading IRI snapshot, please do not kill the process or stop the computer", valuesPath)
	if CurrentTimestamp > 0 {
		logs.Log.Warning("It seems that the the tangle database already exists. Skipping snapshot load from file.")
		return nil
	}
	db.Locker.Lock()
	defer db.Locker.Unlock()

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
		err = db.Put([]byte{db.KEY_SNAPSHOT_DATE}, timestamp, nil,nil)
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
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			logs.Log.Fatalf("read file line error: %v", err)
			return err
		}
		err = loadSpentSnapshot(convert.TrytesToBytes(strings.TrimSpace(line))[:49])
		if err != nil {
			return err
		}
	}

	return nil
}

func loadIRISnapshotValues(valuesPath string) error {
	f, err := os.OpenFile(valuesPath, os.O_RDONLY, os.ModePerm)
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

	rd := bufio.NewReader(f)
	var total int64 = 0
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
		if err != nil { return err }
		total += value
		err = loadValueSnapshot(address, value)
		if err != nil { return err }
	}

	logs.Log.Debugf("Snapshot total value: %v", total)

	return nil
}

/*
Deprecated. Used to load missing address hash bytes from IRI snapshot.
 */
func LoadAddressBytes(valuesPath string) error {
	logs.Log.Debugf("Verifying byte addresses")
	f, err := os.OpenFile(valuesPath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	if err != nil {
		return err
	}

	rd := bufio.NewReader(f)
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
		addressKey := db.GetByteKey(address, db.KEY_ADDRESS_BYTES)
		err = db.PutBytes(addressKey, address, nil, nil)
		if err != nil { return err }
	}

	return nil
}