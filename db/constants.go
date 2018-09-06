package db

import (
	"crypto/md5"
)

const (
	// "Thumbnails" are the md5sum of the data, which is used to identify it. In that way the DB size is reduced

	KEY_FINGERPRINT = byte(0) // Thumbnail of complete received data => to skip the same data package in a short time range (not stored in DB)

	// TRANSACTION SAVING
	KEY_HASH         = byte(1) // []byte -> []byte								Tx Hash thumbnail -> Hash of the Tx
	KEY_TIMESTAMP    = byte(2) // []byte -> int64								Tx Hash thumbnail -> Creation timestamp of the Tx
	KEY_BYTES        = byte(3) // []byte -> []byte								Tx Hash thumbnail -> Raw Tx bytes
	KEY_BUNDLE       = byte(4) // []byte -> int									Bundle Hash thumbnail + Tx Hash thumbnail -> Index
	KEY_ADDRESS      = byte(5) // []byte -> int64								Address thumbnail + Tx Hash thumbnail -> Value of the Tx
	KEY_TAG          = byte(6) // []byte -> nil									Tag thumbnail + Tx Hash thumbnail -> not used
	KEY_VALUE        = byte(7) // []byte -> int64								Tx Hash thumbnail -> Value of the Tx
	KEY_ADDRESS_HASH = byte(8) // []byte -> []byte								Tx Hash thumbnail -> Address of the Tx

	// RELATIONS
	KEY_RELATION = byte(15) // []byte -> []byte									Tx Hash thumbnail -> Trunk Hash thumbnail + Branch Hash thumbnail
	KEY_APPROVEE = byte(16) // []byte -> bool									Trunk/Branch Hash thumbnail + Tx Hash thumbnail -> true (trunk) / false (branch)

	// MILESTONE/CONFIRMATION RELATED
	KEY_MILESTONE       = byte(20) // []byte -> int 							Tx Hash thumbnail of milestone -> Milestone index
	KEY_SOLID_MILESTONE = byte(21) // not used
	KEY_CONFIRMED       = byte(25) // []byte -> int64							Tx Hash thumbnail -> ??? Creation timestamp of the Tx ??? (marker for confirmed Tx) => TODO: Should be confirmation Timestamp
	KEY_TIP             = byte(27) // []byte -> int64							Tx Hash thumbnail -> ??? Creation timestamp of the Tx ??? (marker for a Tip) => TODO: Should be reattachment Timestamp

	// PENDING + UNKNOWN CONFIRMED TRANSACTIONS
	KEY_PENDING_TIMESTAMP = byte(30) // []byte -> int64							Tx Hash thumbnail -> Timestamp of last request
	KEY_PENDING_HASH      = byte(31) // []byte -> []byte						Tx Hash thumbnail -> Hash of the requested Tx
	KEY_PENDING_REQUESTS  = byte(32) // []byte -> int64							Tx Hash thumbnail -> Counts how often the Tx was requested
	KEY_PENDING_CONFIRMED = byte(35) // []byte -> int64							Tx Hash thumbnail -> Timestamp of confirmation
	KEY_PENDING_BUNDLE    = byte(39) // not used

	// PERSISTENT EVENTS
	KEY_EVENT_MILESTONE_PENDING           = byte(50) // []byte -> []byte		Milestone Tx Hash thumbnail -> Milestone Trunk Hash thumbnail
	KEY_EVENT_MILESTONE_PAIR_PENDING      = byte(51) // []byte -> []byte		Milestone Trunk Hash thumbnail -> Milestone Tx Hash thumbnail
	KEY_EVENT_CONFIRMATION_PENDING        = byte(56) // []byte -> int64			Tx Hash thumbnail -> Creation timestamp of the Tx
	KEY_EVENT_BUNDLE_CONFIRMATION_PENDING = byte(57) // not used
	KEY_EVENT_TRIM_PENDING                = byte(58) // []byte -> bool			Tx Hash thumbnail -> bool (marker that this Tx will be trimmed because of snapshot)

	// OTHER
	KEY_BALANCE          = byte(100) // []byte -> int64							Address thumbnail -> Address balance
	KEY_SPENT            = byte(101) // []byte -> bool							Address thumbnail -> Address was spent from
	KEY_ADDRESS_BYTES    = byte(105) // not used
	KEY_SNAPSHOT_BALANCE = byte(120) // []byte -> int64							Address thumbnail -> Spapshotted address balance
	KEY_SNAPSHOT_SPENT   = byte(121) // []byte -> bool							Address thumbnail -> Snapshotted address was spent from
	KEY_SNAPSHOT_CLOCK   = byte(126) // not used
	KEY_SNAPSHOT_LOCK    = byte(127) // byte -> int			 					Timestamp of Snapshot lock
	KEY_SNAPSHOT_FILE    = byte(128) // byte -> string		 					Snapshot filename
	KEY_SNAPSHOT_DATE    = byte(129) // byte -> int								Snapshot Timestamp
	KEY_SNAPSHOTTED      = byte(130) // []byte -> int							Tx Hash thumbnail -> Timestamp of snapshot (marker that Tx was snapshotted)
	KEY_EDGE             = byte(150) // []byte -> int							Tx Hash thumbnail -> Timestamp of snapshot (marker that Tx is over the edge, older than the snapshot)
	KEY_GTTA             = byte(160) // []byte -> int							Tx Hash thumbnail -> Timestamp of snapshot gTTA Walk (marker that Tx was choosen in a gTTA request)
	KEY_TEST             = byte(187) // not used
	KEY_OTHER            = byte(255) // not used
)

// Returns a 16-bytes key based on other key
func AsKey(keyBytes []byte, key byte) []byte {
	b := make([]byte, 16)
	copy(b, keyBytes)
	b[0] = key
	return b
}

// Returns a 16-bytes key based on bytes
func GetByteKey(bytes []byte, key byte) []byte {
	b := md5.Sum(bytes)
	b[0] = key
	return b[:]
}

// Returns a 16-bytes key based on bytes
func GetAddressKey(address []byte, key byte) []byte {
	b := make([]byte, 50)
	b[0] = key
	copy(b[1:], address)
	return b
}
