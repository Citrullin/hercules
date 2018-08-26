package db

import (
	"crypto/md5"
)

// TODO: the descriptions are mostly wrong...
const (
	KEY_FINGERPRINT = byte(0) // hash -> tx.hash trytes

	// TRANSACTION SAVING
	KEY_HASH         = byte(1) // hash -> tx.hash
	KEY_TIMESTAMP    = byte(2) // hash -> time
	KEY_BYTES        = byte(3) // hash -> raw tx trytes
	KEY_BUNDLE       = byte(4) // bundle hash + tx hash -> index
	KEY_ADDRESS      = byte(5) // address hash + hash -> value
	KEY_TAG          = byte(6) // tag hash + hash -> empty
	KEY_VALUE        = byte(7) // hash -> int64
	KEY_ADDRESS_HASH = byte(8) // hash -> address

	// RELATIONS
	KEY_RELATION = byte(15) // hash -> hash+hash
	KEY_APPROVEE = byte(16) // hash + parent hash -> empty

	// MILESTONE/CONFIRMATION RELATED
	KEY_MILESTONE       = byte(20) // hash -> index
	KEY_SOLID_MILESTONE = byte(21) // hash -> index
	KEY_CONFIRMED       = byte(25) // hash -> time
	KEY_TIP             = byte(27) // hash -> time

	// PENDING + UNKNOWN CONFIRMED TRANSACTIONS
	KEY_PENDING_TIMESTAMP = byte(30) // hash -> parent time
	KEY_PENDING_HASH      = byte(31) // hash -> hash
	KEY_PENDING_REQUESTS  = byte(32) // hash -> int
	KEY_PENDING_CONFIRMED = byte(35) // hash -> parent time
	KEY_PENDING_BUNDLE    = byte(39) // hash -> timestamp

	// PERSISTENT EVENTS
	KEY_EVENT_MILESTONE_PENDING           = byte(50) // trunk hash (999 address) -> tx hash
	KEY_EVENT_MILESTONE_PAIR_PENDING      = byte(51) // trunk hash (999 address) -> tx hash
	KEY_EVENT_CONFIRMATION_PENDING        = byte(56) // hash (coo address) -> index
	KEY_EVENT_BUNDLE_CONFIRMATION_PENDING = byte(57)
	KEY_EVENT_TRIM_PENDING                = byte(58) // hash -> bool

	// OTHER
	KEY_BALANCE          = byte(100) // address hash -> int64
	KEY_SPENT            = byte(101) // address hash -> bool
	KEY_ADDRESS_BYTES    = byte(105) // address hash -> hash bytes
	KEY_SNAPSHOT_BALANCE = byte(120) // address hash -> int64
	KEY_SNAPSHOT_SPENT   = byte(121) // address hash -> bool
	KEY_SNAPSHOT_CLOCK   = byte(126) // byte -> int (timestamp)
	KEY_SNAPSHOT_LOCK    = byte(127) // byte -> int (timestamp)
	KEY_SNAPSHOT_FILE    = byte(128) // byte -> string
	KEY_SNAPSHOT_DATE    = byte(129) // byte -> int (timestamp)
	KEY_SNAPSHOTTED      = byte(130) // byte -> int (timestamp)
	KEY_EDGE             = byte(150) // byte -> int (timestamp)
	KEY_GTTA             = byte(160) // hash -> int (timestamp)
	KEY_TEST             = byte(187) // hash -> bool
	KEY_OTHER            = byte(255) // XXXX -> any bytes
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
