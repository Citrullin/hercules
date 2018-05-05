package tangle

import (
	"time"
	"fmt"
	"db"
)

func report () {
	flushTicker := time.NewTicker(reportInterval)
	for range flushTicker.C {
		fmt.Printf("I: %v/%v Saved: %v Discarded: %v \n", incomingProcessed, incoming, saved, discarded)
		fmt.Printf("Queues: SI I/O: %v/%v I: %v O: %v R: %v\n",
			len(srv.Incoming),
			len(srv.Outgoing),
			len(incomingQueue),
			len(outgoingQueue),
			len(requestReplyQueue))
		fmt.Printf("Totals: Conf: %v/%v, TXs: %v, Req: %v, Unknown: %v \n",
			db.Count(db.KEY_CONFIRMED),
			db.Count(db.KEY_EVENT_CONFIRMATION_PENDING),
			db.Count(db.KEY_HASH),
			db.Count(db.KEY_PENDING),
			db.Count(db.KEY_PENDING_CONFIRMED))
		fmt.Printf("Milestone: TXs %v, Pending: %v\n",
			db.Count(db.KEY_MILESTONE),
			db.Count(db.KEY_EVENT_MILESTONE_PENDING))
	}
}
