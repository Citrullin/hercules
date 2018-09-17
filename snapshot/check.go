package snapshot

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"../config"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../utils"
)

/*
Returns if the given timestamp is more recent than the current database snapshot.
*/
func IsNewerThanSnapshot(timestamp int64, dbTx db.Transaction) bool {
	current := GetSnapshotTimestamp(dbTx)
	return timestamp > current
}

/*
Returns if the given timestamp is more recent than the current database snapshot.
*/
func IsEqualOrNewerThanSnapshot(timestamp int64, dbTx db.Transaction) bool {
	current := GetSnapshotTimestamp(dbTx)
	return timestamp >= current
}

/*
Returns whether the current tangle is synchronized
*/
// TODO: this check is too slow on bigger databases. The counters should be moved to memory.
func IsSynchronized() bool {
	return ns.Count(db.Singleton, ns.NamespacePendingConfirmed) < 10 &&
		ns.Count(db.Singleton, ns.NamespaceEventConfirmationPending) < 10 &&
		ns.Count(db.Singleton, ns.NamespaceEventMilestonePending) < 5
}

/*
Checks outstanding pending confirmations that node is beyond the snapshot horizon.
This is just an additional measure to prevent tangle inconsistencies.
*/
func CanSnapshot(timestamp int64) bool {
	return !coding.HasKeyInCategoryWithInt64LowerEqual(db.Singleton, ns.NamespaceEventConfirmationPending, timestamp)
}

func checkDatabaseSnapshot() bool {
	logs.Log.Info("Checking database snapshot integrity")

	total := coding.SumInt64InCategory(db.Singleton, ns.NamespaceSnapshotBalance)
	if total == TOTAL_IOTAS {
		logs.Log.Info("Database snapshot integrity check passed")
		return true
	} else {
		logs.Log.Errorf("Database snapshot integrity check failed: %v should be %v", total, TOTAL_IOTAS)
		logs.Log.Fatal("The database is in an inconsistent state now :(. Dying...")
		return false
	}
}

func checkSnapshotFile(path string) (timestamp int64, err error) {
	// Check timestamp
	header, err := LoadHeader(path)

	if err != nil {
		return 0, err
	}

	logs.Log.Debugf("Loaded Header v.%v, timestamp: %v (%v)", header.Version, utils.GetHumanReadableTime(header.Timestamp), header.Timestamp)

	timestamp = header.Timestamp

	current, err := coding.GetInt64(db.Singleton, []byte{ns.NamespaceSnapshotDate})
	if err == nil && current > timestamp {
		logs.Log.Errorf(
			"The current snapshot (%v) is more recent than the one being loaded (%v)!",
			time.Unix(current, 0),
			time.Unix(timestamp, 0))
		return 0, errors.New("current snapshot more recent")
	}

	err = checkSnapshotFileIntegrity(path)
	if err != nil {
		return 0, err
	}

	return timestamp, nil
}

func checkSnapshotFileIntegrity(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return err
	}
	defer f.Close()

	var checkingSpent = false
	var total int64 = 0
	var totalSpent int64 = 0
	var firstLine = true

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
				if firstLine && len(tokens) < 2 {
					// Header
					continue
				}
				value, err := strconv.ParseInt(tokens[1], 10, 64)
				if err != nil {
					logs.Log.Errorf("Error parsing address value: %v => %v", tokens[1], err)
					return err
				}
				total += value
			}
		}
		firstLine = false
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
If yes, it checks if there is a snapshot lock filename.
If filename is present, run LoadSnapshot, otherwise run MakeSnapshot.
*/
func checkPendingSnapshot() {
	timestamp, filename := IsLocked(nil)
	if timestamp >= 0 {
		if len(filename) > 0 {
			newFilename := config.AppConfig.GetString("snapshots.loadFile")
			if len(newFilename) > 0 {
				filename = newFilename
			}
			logs.Log.Info("Found pending snapshot lock. Trying to continue... ", filename)
			LoadSnapshot(filename)
		} else {
			logs.Log.Info("Found pending snapshot lock. Trying to continue... ", timestamp)
			MakeSnapshot(timestamp, "")
		}
	}
}

/*
Returns whether a transaction from the database can be snapshotted
*/
func canBeSnapshotted(key []byte, dbTx db.Transaction) bool {
	return dbTx.HasKey(ns.Key(key, ns.NamespaceConfirmed)) &&
		!dbTx.HasKey(ns.Key(key, ns.NamespaceEventTrimPending)) &&
		!dbTx.HasKey(ns.Key(key, ns.NamespaceSnapshotted))
}
