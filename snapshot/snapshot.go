package snapshot

import (
	"sync"
	"time"

	"../config"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../utils"
)

const (
	SNAPSHOT_SEPARATOR         = "==="
	TIMESTAMP_MIN              = 1525017600
	MIN_SPENT_ADDRESSES        = 521970
	MAX_LATEST_TRANSACTION_AGE = 300
)

var (
	TOTAL_IOTAS     int64 = 2779530283277761
	keySnapshotDate       = []byte{ns.NamespaceSnapshotDate}
	keySnapshotLock       = []byte{ns.NamespaceSnapshotLock}
	keySnapshotFile       = []byte{ns.NamespaceSnapshotFile}

	snapshotTicker            *time.Ticker
	snapshotTickerQuit        = make(chan struct{})
	edgeTransactions          = make(chan *[]byte, 10000000)
	edgeTransactionsWaitGroup = &sync.WaitGroup{}
	edgeTransactionsQueueQuit = make(chan struct{})
	CurrentTimestamp          int64
	SnapshotInProgress        = false
	SnapshotWaitGroup         = &sync.WaitGroup{}
	lowEndDevice              = false
	timeLeftToSnapshotSeconds int64
	ended                     = false
)

func Start() {
	logs.Log.Debug("Loading snapshots module")

	lowEndDevice = config.AppConfig.GetBool("light")
	CurrentTimestamp = GetSnapshotTimestamp(nil)
	logs.Log.Infof("Current snapshot timestamp: %v", CurrentTimestamp)

	// TODO Refactor this function name and content so it is more readable
	checkPendingSnapshot()

	snapshotPeriod := config.AppConfig.GetInt64("snapshots.period")
	snapshotInterval := config.AppConfig.GetInt64("snapshots.interval")
	ensureSnapshotIsUpToDate(snapshotInterval, snapshotPeriod)

	loadSnapshotFiles()

	go startAutosnapshots(snapshotInterval, snapshotPeriod)
	go trimTXRunner()
}

func End() {
	ended = true

	if snapshotTicker != nil {
		snapshotTicker.Stop()
		close(snapshotTickerQuit)
	}
	close(edgeTransactionsQueueQuit)

	SnapshotWaitGroup.Wait()

	edgeTransactionsWaitGroup.Wait()
	close(edgeTransactions)

	logs.Log.Debug("Snapshot module exited")
}

func loadSnapshotFiles() {
	snapshotToLoad := config.AppConfig.GetString("snapshots.loadFile")
	iri1 := config.AppConfig.GetString("snapshots.loadIRIFile")
	iri2 := config.AppConfig.GetString("snapshots.loadIRISpentFile")
	iriTimestamp := config.AppConfig.GetInt64("snapshots.loadIRITimestamp")
	if len(snapshotToLoad) > 0 {
		LoadSnapshot(snapshotToLoad)
	} else if len(iri1) > 0 && len(iri2) > 0 && iriTimestamp > 0 {
		LoadIRISnapshot(iri1, iri2, iriTimestamp)
	}

	if !checkDatabaseSnapshot() {
		logs.Log.Fatalf("Database is in an inconsistent state. Try deleting it and loading a snapshot.")
	}
}

/*
Sets the current snapshot date in the database
*/
func SetSnapshotTimestamp(timestamp int64, tx db.Transaction) error {
	if tx == nil {
		tx = db.Singleton.NewTransaction(true)
		defer tx.Commit()
	}
	err := coding.PutInt64(tx, keySnapshotDate, timestamp)
	if err == nil {
		CurrentTimestamp = timestamp
	}
	return err
}

/*
Returns timestamp if snapshot lock is present. Otherwise negative number.
If this is a file lock (snapshot being loaded from a file)
*/
func IsLocked(tx db.Transaction) (timestamp int64, filename string) {
	if tx == nil {
		tx = db.Singleton.NewTransaction(false)
		defer tx.Discard()
	}
	return GetSnapshotLock(tx), GetSnapshotFileLock(tx)
}

/*
Creates a snapshot lock in the database
*/
func Lock(timestamp int64, filename string, tx db.Transaction) error {
	if tx == nil {
		tx = db.Singleton.NewTransaction(true)
		defer tx.Commit()
	}

	SnapshotInProgress = true
	SnapshotWaitGroup.Add(1)
	err := coding.PutInt64(tx, keySnapshotLock, timestamp)
	if err != nil {
		return err
	}
	return coding.PutString(tx, keySnapshotFile, filename)
}

/*
Removes a snapshot lock in the database
*/
func Unlock(tx db.Transaction) error {
	SnapshotInProgress = false
	SnapshotWaitGroup.Done()
	err := tx.Remove(keySnapshotLock)
	if err != nil {
		return err
	}
	return tx.Remove(keySnapshotFile)
}

/*
Returns the date unix timestamp of the last snapshot
*/
func GetSnapshotLock(tx db.Transaction) int64 {
	if tx == nil {
		tx = db.Singleton.NewTransaction(false)
		defer tx.Discard()
	}

	timestamp, err := coding.GetInt64(tx, keySnapshotLock)
	if err != nil {
		return -1
	}
	return timestamp
}

/*
Returns the date unix timestamp of the last snapshot
*/
func GetSnapshotTimestamp(tx db.Transaction) int64 {
	if CurrentTimestamp > 0 {
		return CurrentTimestamp
	}

	if tx == nil {
		tx = db.Singleton.NewTransaction(false)
		defer tx.Discard()
	}

	timestamp, err := coding.GetInt64(tx, keySnapshotDate)
	if err != nil {
		return -1
	}

	CurrentTimestamp = timestamp
	return timestamp
}

/*
Returns the date unix timestamp of the last snapshot
*/
func GetSnapshotFileLock(tx db.Transaction) string {
	if tx == nil {
		tx = db.Singleton.NewTransaction(false)
		defer tx.Discard()
	}

	filename, err := coding.GetString(tx, keySnapshotFile)
	if err != nil {
		return ""
	}
	return filename
}

/*
Starts a periodic snapshot runner
*/
func startAutosnapshots(snapshotInterval, snapshotPeriod int64) {
	if snapshotInterval <= 0 {
		return
	}

	// Allows hercules to carry time left to snapshots through hercules restarts
	if timeLeftToSnapshotSeconds > 0 {
		snapshotTimestamp := getNextSnapshotTimestamp(snapshotPeriod)
		snapshotTimeHumanReadable := utils.GetHumanReadableTime(snapshotTimestamp)
		logs.Log.Infof("The next automatic snapshot will be done at '%s', keeping the past %v hours.", snapshotTimeHumanReadable)

		t := time.NewTicker(time.Duration(timeLeftToSnapshotSeconds) * time.Second)
		<-t.C // Waits until it ticks
		t.Stop()

		MakeSnapshot(snapshotTimestamp, "")
	}

	logs.Log.Infof("Automatic snapshots will be done every %v hours, keeping the past %v hours.", snapshotInterval, snapshotPeriod)

	snapshotTicker = time.NewTicker(time.Duration(60*snapshotInterval) * time.Minute)
	for {
		select {
		case <-snapshotTickerQuit:
			return

		case <-snapshotTicker.C:
			if ended {
				break
			}
			logs.Log.Info("Starting automatic snapshot...")
			if !SnapshotInProgress {
				snapshotTimestamp := getNextSnapshotTimestamp(snapshotPeriod)
				MakeSnapshot(snapshotTimestamp, "")
			} else {
				logs.Log.Warning("D'oh! A snapshot is already in progress. Skipping current run.")
			}
		}
	}
}

/*
When a snapshot is missing:
- If node is synced: Makes a snapshot when last snapshot made is older than the snapshotPeriod
- If node is not synced: TODO Get snapshot from synced hercules neighbor
*/
func ensureSnapshotIsUpToDate(snapshotInterval, snapshotPeriod int64) {
	if snapshotInterval <= 0 || CurrentTimestamp <= 0 {
		return
	}

	testNextSnapshotTimeStamp := getNextSnapshotTimestamp(snapshotPeriod)
	snapshotMissingSuspicion := CurrentTimestamp < testNextSnapshotTimeStamp
	if snapshotMissingSuspicion {
		timestampDifference := testNextSnapshotTimeStamp - CurrentTimestamp

		snapshotIntervalSeconds := snapshotInterval * 3600
		snapshotMissing := timestampDifference >= snapshotIntervalSeconds
		if snapshotMissing {
			if IsSynchronized() {
				logs.Log.Warningf("Last snapshot was created before the configured snapshot interval: Every %v hours. Making a new snapshot. This can take a few minutes.", snapshotInterval)
				MakeSnapshot(testNextSnapshotTimeStamp, "")
			} else {
				logs.Log.Warningf("Last snapshot is older than expected (more than %v hours) and node is not synced.", snapshotInterval)
				// TODO Get snapshot from synced hercules neighbor
			}
		} else {
			timeLeftToSnapshotSeconds = timestampDifference - snapshotIntervalSeconds
		}
	}
}

func getNextSnapshotTimestamp(snapshotPeriod int64) int64 {
	snapshotPeriodSeconds := snapshotPeriod * 3600
	return time.Now().Unix() - snapshotPeriodSeconds
}
