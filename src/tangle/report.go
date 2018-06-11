package tangle

import (
	"time"
	"db"
	"logs"
)

func Report () {
	logs.Log.Debugf("INCOMING:      In: %v, Queued: %v, Pending: %v \n",
		incoming,
		incomingProcessed,
		len(txQueue))
	logs.Log.Debugf("OUTGOING:      %v", outgoing)
	logs.Log.Debugf("SERVER QUEUE:  In: %v, Out: %v \n",
		len(srv.Incoming),
		len(srv.Outgoing))
	requestLocker.RLock()
	for i, queue := range requestQueues {
		logs.Log.Debugf("PEER I QUEUE:  %v - %v \n", i, len(*queue))
	}
	requestLocker.RUnlock()
	replyLocker.RLock()
	for i, queue := range replyQueues {
		logs.Log.Debugf("PEER O QUEUE:  %v - %v \n", i, len(*queue))
	}
	replyLocker.RUnlock()
	logs.Log.Infof("TRANSACTIONS:  %v, Requests: %v", totalTransactions, len(pendingRequests))
	logs.Log.Infof("CONFIRMATIONS: %v, Pending: %v, Unknown: %v",
		totalConfirmations,
		db.Count(db.KEY_EVENT_CONFIRMATION_PENDING),
		db.Count(db.KEY_PENDING_CONFIRMED))
	logs.Log.Debugf("PENDING TRIMS: %v", db.Count(db.KEY_EVENT_TRIM_PENDING))
	logs.Log.Infof("MILESTONES:    Current: %v, Confirmed: %v, Pending: %v (%v) \n",
		LatestMilestone.Index,
		db.Count(db.KEY_MILESTONE),
		db.Count(db.KEY_EVENT_MILESTONE_PENDING),
		len(pendingMilestoneQueue))
	logs.Log.Infof("TIPS:          %v\n", db.Count(db.KEY_TIP))
}

func report () {
	Report()
	flushTicker := time.NewTicker(reportInterval)
	for range flushTicker.C {
		Report()
	}
}