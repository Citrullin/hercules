package tangle

import (
	"sync"
	"time"

	"../db"
	"../db/ns"
	"../logs"
	"../server"
)

var (
	tangleReportTicker          *time.Ticker
	tangleReportTickerWaitGroup = &sync.WaitGroup{}
	tangleReportTickerQuit      = make(chan struct{})
)

func Report() {
	logs.Log.Debugf("TX IN/OUT:     %v, %v", server.TotalIncTx, outgoing)
	logs.Log.Debugf("SERVER I/O Q:  %v, %v", len(srv.Incoming), len(srv.Outgoing))
	logs.Log.Infof("TRANSACTIONS:  %v, Requests: %v (%v)", totalTransactions,
		ns.Count(db.Singleton, ns.NamespacePendingHash), len(PendingRequests))
	logs.Log.Infof("CONFIRMATIONS: %v, Pending: %v (%v), Unknown: %v", totalConfirmations,
		ns.Count(db.Singleton, ns.NamespaceEventConfirmationPending),
		len(confirmQueue),
		ns.Count(db.Singleton, ns.NamespacePendingConfirmed))
	logs.Log.Debugf("PENDING TRIMS: %v", ns.Count(db.Singleton, ns.NamespaceEventTrimPending))
	logs.Log.Infof("MILESTONES:    Current: %v, Confirmed: %v, Pending: %v, In pending queue %v",
		LatestMilestone.Index,
		ns.Count(db.Singleton, ns.NamespaceMilestone),
		ns.Count(db.Singleton, ns.NamespaceEventMilestonePending),
		len(pendingMilestoneQueue))
	logs.Log.Infof("TIPS:          %v", ns.Count(db.Singleton, ns.NamespaceTip))
}

func report() {
	tangleReportTickerWaitGroup.Add(1)
	defer tangleReportTickerWaitGroup.Done()

	Report()

	tangleReportTicker = time.NewTicker(reportInterval)
	for {
		select {
		case <-tangleReportTickerQuit:
			return

		case <-tangleReportTicker.C:
			if ended {
				break
			}
			Report()
		}
	}
}
