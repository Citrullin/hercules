package tangle

import (
	"time"
	"db"
	"logs"
)

func report () {
	flushTicker := time.NewTicker(reportInterval)
	for range flushTicker.C {
		logs.Log.Debugf("INCOMING:      In: %v, Processed: %v \n",
			incoming,
			incomingProcessed)
		logs.Log.Debugf("QUEUES:        Server I/O: %v/%v, In: %v, Out: %v \n",
			len(srv.Incoming),
			len(srv.Outgoing),
			len(incomingQueue),
			len(outgoingQueue))
		for i, queue := range requestReplyQueues {
			logs.Log.Debugf("REPLY QUEUE:        %v - %v \n", i, len(*queue))
		}
		oldest, t := oldestPending()
		PendingsLocker.Lock()
		pendingsLen := len(pendings)
		PendingsLocker.Unlock()
		logs.Log.Debugf("TRANSACTIONS:  %v, Requests: %v (%v) Oldest: %v - %v \n",
			db.Count(db.KEY_HASH),
			db.Count(db.KEY_PENDING_HASH),
			pendingsLen, t/1000000000.0, oldest)
		logs.Log.Debugf("CONFIRMATIONS: %v, Pending: %v, Unknown: %v \n",
			db.Count(db.KEY_CONFIRMED),
			db.Count(db.KEY_EVENT_CONFIRMATION_PENDING),
			db.Count(db.KEY_PENDING_CONFIRMED))
		logs.Log.Debugf("MILESTONES:    Current: %v, Confirmed: %v, Pending: %v \n",
			milestones[db.KEY_MILESTONE].Index,
			db.Count(db.KEY_MILESTONE),
			db.Count(db.KEY_EVENT_MILESTONE_PENDING))
		logs.Log.Debugf("TIPS:          %v\n", db.Count(db.KEY_TIP))
	}
}
