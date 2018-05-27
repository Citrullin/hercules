package tangle

import (
	"time"
	"db"
	"logs"
)

func report () {
	flushTicker := time.NewTicker(reportInterval)
	for range flushTicker.C {
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
		// TODO: have, in-memory counters instead of using db.Count
		logs.Log.Debugf("TRANSACTIONS:  %v, Requests: %v (%v) \n",
			db.Count(db.KEY_HASH),
			db.Count(db.KEY_PENDING_HASH), len(pendingRequests))
		logs.Log.Debugf("CONFIRMATIONS: %v, Pending: %v, Unknown: %v \n",
			db.Count(db.KEY_CONFIRMED),
			db.Count(db.KEY_EVENT_CONFIRMATION_PENDING),
			db.Count(db.KEY_PENDING_CONFIRMED))
		logs.Log.Debugf("MILESTONES:    Current: %v, Confirmed: %v, Pending: %v (%v) \n",
			milestones[db.KEY_MILESTONE].Index,
			db.Count(db.KEY_MILESTONE),
			db.Count(db.KEY_EVENT_MILESTONE_PENDING),
			len(pendingMilestoneQueue))
		logs.Log.Debugf("TIPS:          %v\n", db.Count(db.KEY_TIP))
		logs.Log.Debugf("PENDING TRIMS: %v,",
			db.Count(db.KEY_EVENT_TRIM_PENDING))
	}
}

var pendingPairs [][]byte