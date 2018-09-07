package tangle

import (
	"time"

	"../db"
	"../db/ns"
	"../logs"
)

func Report() {
	logs.Log.Debugf("TX IN/OUT:     %v, %v", incoming, outgoing)
	logs.Log.Debugf("SERVER I/O Q:  %v, %v \n",
		len(srv.Incoming),
		len(srv.Outgoing))
	logs.Log.Infof("TRANSACTIONS:  %v, Requests: %v (%v)",
		totalTransactions,
		db.Singleton.CountKeyCategory(ns.NamespacePendingHash),
		len(PendingRequests))
	logs.Log.Infof("CONFIRMATIONS: %v, Pending: %v (%v), Unknown: %v",
		totalConfirmations,
		db.Singleton.CountKeyCategory(ns.NamespaceEventConfirmationPending),
		len(confirmQueue),
		db.Singleton.CountKeyCategory(ns.NamespacePendingConfirmed))
	logs.Log.Debugf("PENDING TRIMS: %v", db.Singleton.CountKeyCategory(ns.NamespaceEventTrimPending))
	logs.Log.Infof("MILESTONES:    Current: %v, Confirmed: %v, Pending: %v (%v) \n",
		LatestMilestone.Index,
		db.Singleton.CountKeyCategory(ns.NamespaceMilestone),
		db.Singleton.CountKeyCategory(ns.NamespaceEventMilestonePending),
		len(pendingMilestoneQueue))
	logs.Log.Infof("TIPS:          %v\n", db.Singleton.CountKeyCategory(ns.NamespaceTip))
}

func report() {
	Report()
	tangleReportTicker := time.NewTicker(reportInterval)
	for range tangleReportTicker.C {
		Report()
	}
}
