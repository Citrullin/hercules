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
	iriFile := config.AppConfig.GetString("snapshots.loadIRIFile")
	spentFiles := config.AppConfig.GetStringSlice("snapshots.loadIRISpentFiles")
	iriTimestamp := config.AppConfig.GetInt64("snapshots.loadIRITimestamp")
	if len(snapshotToLoad) > 0 {
		LoadSnapshot(snapshotToLoad)
	} else if len(iriFile) > 0 && len(spentFiles) > 0 && iriTimestamp > 0 {
		LoadIRISnapshot(iriFile, spentFiles, iriTimestamp)
	}

	if !checkDatabaseSnapshot() {
		logs.Log.Fatalf("Database is in an inconsistent state. Try deleting it and loading a snapshot.")
	}
}

/*
Sets the current snapshot date in the database
*/
func SetSnapshotTimestamp(timestamp int64, dbTx db.Transaction) error {
	if dbTx == nil {
		dbTx = db.Singleton.NewTransaction(true)
		defer dbTx.Commit()
	}
	err := coding.PutInt64(dbTx, keySnapshotDate, timestamp)
	if err == nil {
		CurrentTimestamp = timestamp
	}
	return err
}

/*
Returns timestamp if snapshot lock is present. Otherwise negative number.
If this is a file lock (snapshot being loaded from a file)
*/
func IsLocked(dbTx db.Transaction) (timestamp int64, filename string) {
	if dbTx == nil {
		dbTx = db.Singleton.NewTransaction(false)
		defer dbTx.Discard()
	}
	return GetSnapshotLock(dbTx), GetSnapshotFileLock(dbTx)
}

/*
Creates a snapshot lock in the database
*/
func Lock(timestamp int64, filename string, dbTx db.Transaction) error {
	if dbTx == nil {
		dbTx = db.Singleton.NewTransaction(true)
		defer dbTx.Commit()
	}

	SnapshotInProgress = true
	SnapshotWaitGroup.Add(1)
	err := coding.PutInt64(dbTx, keySnapshotLock, timestamp)
	if err != nil {
		return err
	}
	return coding.PutString(dbTx, keySnapshotFile, filename)
}

/*
Removes a snapshot lock in the database
*/
func Unlock(dbTx db.Transaction) error {
	SnapshotInProgress = false
	SnapshotWaitGroup.Done()
	err := dbTx.Remove(keySnapshotLock)
	if err != nil {
		return err
	}
	return dbTx.Remove(keySnapshotFile)
}

/*
Returns the date unix timestamp of the last snapshot
*/
func GetSnapshotLock(dbTx db.Transaction) int64 {
	if dbTx == nil {
		dbTx = db.Singleton.NewTransaction(false)
		defer dbTx.Discard()
	}

	timestamp, err := coding.GetInt64(dbTx, keySnapshotLock)
	if err != nil {
		return -1
	}
	return timestamp
}

/*
Returns the date unix timestamp of the last snapshot
*/
func GetSnapshotTimestamp(dbTx db.Transaction) int64 {
	if CurrentTimestamp > 0 {
		return CurrentTimestamp
	}

	if dbTx == nil {
		dbTx = db.Singleton.NewTransaction(false)
		defer dbTx.Discard()
	}

	timestamp, err := coding.GetInt64(dbTx, keySnapshotDate)
	if err != nil {
		return -1
	}

	CurrentTimestamp = timestamp
	return timestamp
}

/*
Returns the date unix timestamp of the last snapshot
*/
func GetSnapshotFileLock(dbTx db.Transaction) string {
	if dbTx == nil {
		dbTx = db.Singleton.NewTransaction(false)
		defer dbTx.Discard()
	}

	filename, err := coding.GetString(dbTx, keySnapshotFile)
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
