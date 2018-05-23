package snapshot

import (
    "time"
    "logs"
)

const (
	SNAPSHOT_SEPARATOR = "==="
    TOTAL_IOTAS = 2779530283277761
    TIMESTAMP_MIN = 1525017600
    WAIT_SNAPSHOT_DURATION = time.Duration(3) * time.Second
    MAX_SNAPSHOT_TIME = -(time.Duration(1) * time.Hour) // Maximal one hour age of snapshot
    MIN_SPENT_ADDRESSES = 521970
)

var edgeTransactions chan *[]byte


func OnSnapshotsLoad () {
    logs.Log.Debug("Loading snapshots module")
    edgeTransactions = make(chan *[]byte)
    go trimTXRunner()
}
