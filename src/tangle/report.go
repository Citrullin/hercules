package tangle

import (
	"time"
	"db"
	"log"
)

func report () {
	flushTicker := time.NewTicker(reportInterval)
	for range flushTicker.C {
		log.Printf("QUEUES:        Server I/O: %v/%v, In: %v, Out: %v, Reply: %v \n",
			len(srv.Incoming),
			len(srv.Outgoing),
			len(incomingQueue),
			len(outgoingQueue),
			len(requestReplyQueue))
		log.Printf("TRANSACTIONS:  %v, Requests: %v \n",
			db.Count(db.KEY_HASH),
			db.Count(db.KEY_PENDING))
		log.Printf("CONFIRMATIONS: %v, Pending: %v, Unknown: %v \n",
			db.Count(db.KEY_CONFIRMED),
			db.Count(db.KEY_EVENT_CONFIRMATION_PENDING),
			db.Count(db.KEY_PENDING_CONFIRMED))
		log.Printf("MILESTONES:    Current: %v, Confirmed: %v, Pending: %v \n",
			milestones[db.KEY_MILESTONE].Index,
			db.Count(db.KEY_MILESTONE),
			db.Count(db.KEY_EVENT_MILESTONE_PENDING))
		log.Printf("TIPS:          %v\n", db.Count(db.KEY_TIP))
		log.Println("")
	}
}
