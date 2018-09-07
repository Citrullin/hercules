package ns

const (
	// "Thumbnails" are the md5sum of the data, which is used to identify it. In that way the DB size is reduced

	NamespaceFingerprint = byte(0) // Thumbnail of complete received data => to skip the same data package in a short time range (not stored in DB)

	// TRANSACTION SAVING
	NamespaceHash        = byte(1) // []byte -> []byte								Tx Hash thumbnail -> Hash of the Tx
	NamespaceTimestamp   = byte(2) // []byte -> int64								Tx Hash thumbnail -> Creation timestamp of the Tx
	NamespaceBytes       = byte(3) // []byte -> []byte								Tx Hash thumbnail -> Raw Tx bytes
	NamespaceBundle      = byte(4) // []byte -> int									Bundle Hash thumbnail + Tx Hash thumbnail -> Index
	NamespaceAddress     = byte(5) // []byte -> int64								Address thumbnail + Tx Hash thumbnail -> Value of the Tx
	NamespaceTag         = byte(6) // []byte -> nil									Tag thumbnail + Tx Hash thumbnail -> not used
	NamespaceValue       = byte(7) // []byte -> int64								Tx Hash thumbnail -> Value of the Tx
	NamespaceAddressHash = byte(8) // []byte -> []byte								Tx Hash thumbnail -> Address of the Tx

	// RELATIONS
	NamespaceRelation = byte(15) // []byte -> []byte									Tx Hash thumbnail -> Trunk Hash thumbnail + Branch Hash thumbnail
	NamespaceApprovee = byte(16) // []byte -> bool									Trunk/Branch Hash thumbnail + Tx Hash thumbnail -> true (trunk) / false (branch)

	// MILESTONE/CONFIRMATION RELATED
	NamespaceMilestone      = byte(20) // []byte -> int 							Tx Hash thumbnail of milestone -> Milestone index
	NamespaceSolidMilestone = byte(21) // not used
	NamespaceConfirmed      = byte(25) // []byte -> int64							Tx Hash thumbnail -> ??? Creation timestamp of the Tx ??? (marker for confirmed Tx) => TODO: Should be confirmation Timestamp
	NamespaceTip            = byte(27) // []byte -> int64							Tx Hash thumbnail -> ??? Creation timestamp of the Tx ??? (marker for a Tip) => TODO: Should be reattachment Timestamp

	// PENDING + UNKNOWN CONFIRMED TRANSACTIONS
	NamespacePendingTimestamp = byte(30) // []byte -> int64							Tx Hash thumbnail -> Timestamp of last request
	NamespacePendingHash      = byte(31) // []byte -> []byte						Tx Hash thumbnail -> Hash of the requested Tx
	NamespacePendingRequests  = byte(32) // []byte -> int64							Tx Hash thumbnail -> Counts how often the Tx was requested
	NamespacePendingConfirmed = byte(35) // []byte -> int64							Tx Hash thumbnail -> Timestamp of confirmation
	NamespacePendingBundle    = byte(39) // not used

	// PERSISTENT EVENTS
	NamespaceEventMilestonePending          = byte(50) // []byte -> []byte		Milestone Tx Hash thumbnail -> Milestone Trunk Hash thumbnail
	NamespaceEventMilestonePairPending      = byte(51) // []byte -> []byte		Milestone Trunk Hash thumbnail -> Milestone Tx Hash thumbnail
	NamespaceEventConfirmationPending       = byte(56) // []byte -> int64			Tx Hash thumbnail -> Creation timestamp of the Tx
	NamespaceEventBundleConfirmationPending = byte(57) // not used
	NamespaceEventTrimPending               = byte(58) // []byte -> bool			Tx Hash thumbnail -> bool (marker that this Tx will be trimmed because of snapshot)

	// OTHER
	NamespaceBalance         = byte(100) // []byte -> int64							Address thumbnail -> Address balance
	NamespaceSpent           = byte(101) // []byte -> bool							Address thumbnail -> Address was spent from
	NamespaceAddressBytes    = byte(105) // not used
	NamespaceSnapshotBalance = byte(120) // []byte -> int64							Address thumbnail -> Spapshotted address balance
	NamespaceSnapshotSpent   = byte(121) // []byte -> bool							Address thumbnail -> Snapshotted address was spent from
	NamespaceSnapshotClock   = byte(126) // not used
	NamespaceSnapshotLock    = byte(127) // byte -> int			 					Timestamp of Snapshot lock
	NamespaceSnapshotFile    = byte(128) // byte -> string		 					Snapshot filename
	NamespaceSnapshotDate    = byte(129) // byte -> int								Snapshot Timestamp
	NamespaceSnapshotted     = byte(130) // []byte -> int							Tx Hash thumbnail -> Timestamp of snapshot (marker that Tx was snapshotted)
	NamespaceEdge            = byte(150) // []byte -> int							Tx Hash thumbnail -> Timestamp of snapshot (marker that Tx is over the edge, older than the snapshot)
	NamespaceGTTA            = byte(160) // []byte -> int							Tx Hash thumbnail -> Timestamp of snapshot gTTA Walk (marker that Tx was choosen in a gTTA request)
	NamespaceTest            = byte(187) // not used
	NamespaceOther           = byte(255) // not used
)
