package snapshot

import (
    "time"
    "logs"
    "github.com/spf13/viper"
    "db"
    "github.com/dgraph-io/badger"
)

const (
	SNAPSHOT_SEPARATOR = "==="
    TOTAL_IOTAS = 2779530283277761
    TIMESTAMP_MIN = 1525017600
    WAIT_SNAPSHOT_DURATION = time.Duration(3) * time.Second
    MAX_SNAPSHOT_TIME = -(time.Duration(1) * time.Hour) // Maximal one hour age of snapshot
    MIN_SPENT_ADDRESSES = 521970
)

var keySnapshotDate = []byte{db.KEY_SNAPSHOT_DATE}
var keySnapshotLock = []byte{db.KEY_SNAPSHOT_LOCK}
var keySnapshotFile = []byte{db.KEY_SNAPSHOT_FILE}

var edgeTransactions chan *[]byte
var config *viper.Viper
var CurrentTimestamp = 0

func Start(cfg *viper.Viper) {
    config = cfg
    logs.Log.Debug("Loading snapshots module")
    edgeTransactions = make(chan *[]byte, 10000000)

    CurrentTimestamp = GetSnapshotTimestamp(nil)

    go trimTXRunner()

    // TODO: remove this:
    logs.Log.Debugf("CONFIRMATIONS: %v, Pending: %v, Unknown: %v \n",
        db.Count(db.KEY_CONFIRMED),
        db.Count(db.KEY_EVENT_CONFIRMATION_PENDING),
        db.Count(db.KEY_PENDING_CONFIRMED))
    //err := MakeSnapshot(1527310000)
    //logs.Log.Debug("saveSnapshot result:", err)

    checkPendingSnapshot()

    snapshotToLoad := config.GetString("snapshots.loadFile")
    if len(snapshotToLoad) > 0 {
        LoadSnapshot(snapshotToLoad)
    }

    if !checkDatabaseSnapshot() {
        logs.Log.Fatalf("Database is in an inconsistent state. Try deleting it and loading a snapshot.")
    }

    //now := time.Now()
    //weekAgo := now.Add(-time.Duration(24*7) * time.Hour)
    //logs.Log.Warningf("One week ago (%v): %v", weekAgo, weekAgo.Unix())
    //err := MakeSnapshot(1526711179)
    //logs.Log.Warning("saveSnapshot result:", err)

    //loadIRISnapshot("snapshotMainnet.txt","previousEpochsSpentAddresses.txt", 1525017600)
    // err := SaveSnapshot(config.GetString("snapshots.path"))
    //err := LoadSnapshot("snapshots/1525017600.snap")
    //logs.Log.Warning("saveSnapshot result:", err)
}

/*
Sets the current snapshot date in the database
*/
func SetSnapshotTimestamp(timestamp int, txn *badger.Txn) error {
    err := db.Put(keySnapshotDate, timestamp, nil, txn)
    if err == nil {
        CurrentTimestamp = timestamp
    }
    return err
}


/*
Returns timestamp if snapshot lock is present. Otherwise negative number.
If this is a file lock (snapshot being loaded from a file)
 */
func IsLocked (txn *badger.Txn) (timestamp int, filename string) {
    return GetSnapshotLock(txn), GetSnapshotFileLock(txn)
}

/*
Creates a snapshot lock in the database
*/
func Lock(timestamp int, filename string, txn *badger.Txn) error {
    err := db.Put(keySnapshotLock, timestamp, nil, txn)
    if err != nil { return err }
    return db.Put(keySnapshotFile, filename, nil, txn)
}

/*
Removes a snapshot lock in the database
*/
func Unlock(txn *badger.Txn) error {
    err := db.Remove(keySnapshotLock, txn)
    if err != nil { return err }
    return db.Remove(keySnapshotFile, txn)
}

/*
Returns the date unix timestamp of the last snapshot
 */
func GetSnapshotLock (txn *badger.Txn) int {
    timestamp, err := db.GetInt(keySnapshotLock, txn)
    if err != nil { return -1 }
    return timestamp
}

/*
Returns the date unix timestamp of the last snapshot
 */
func GetSnapshotTimestamp(txn *badger.Txn) int {
    if CurrentTimestamp > 0 {
        return CurrentTimestamp
    }

    timestamp, err := db.GetInt(keySnapshotDate, txn)
    if err != nil { return -1 }
    return timestamp
}

/*
Returns the date unix timestamp of the last snapshot
 */
func GetSnapshotFileLock (txn *badger.Txn) string {
    filename, err := db.GetString(keySnapshotFile, txn)
    if err != nil { return "" }
    return filename
}