package tangle

import (
	"time"

	"../db"
	"../logs"
)

func Report() {
	logs.Log.Debugf("TX IN/OUT:     %v, %v", incoming, outgoing)
	logs.Log.Debugf("SERVER I/O Q:  %v, %v \n",
		len(srv.Incoming),
		len(srv.Outgoing))
	logs.Log.Infof("TRANSACTIONS:  %v, Requests: %v (%v)",
		totalTransactions,
		db.Count(db.KEY_PENDING_HASH),
		len(pendingRequests))
	logs.Log.Infof("CONFIRMATIONS: %v, Pending: %v (%v), Unknown: %v",
		totalConfirmations,
		db.Count(db.KEY_EVENT_CONFIRMATION_PENDING),
		len(confirmQueue),
		db.Count(db.KEY_PENDING_CONFIRMED))
	logs.Log.Debugf("PENDING TRIMS: %v", db.Count(db.KEY_EVENT_TRIM_PENDING))
	logs.Log.Infof("MILESTONES:    Current: %v, Confirmed: %v, Pending: %v (%v) \n",
		LatestMilestone.Index,
		db.Count(db.KEY_MILESTONE),
		db.Count(db.KEY_EVENT_MILESTONE_PENDING),
		len(pendingMilestoneQueue))
	logs.Log.Infof("TIPS:          %v\n", db.Count(db.KEY_TIP))
}

func report() {
	Report()
	flushTicker := time.NewTicker(reportInterval)
	for range flushTicker.C {
		Report()
	}
}
