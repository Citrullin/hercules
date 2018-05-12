package tangle

import (
	"time"
	"db"
	"github.com/dgraph-io/badger"
	"logs"
	"convert"
	"bytes"
	"encoding/gob"
	"server"
)

func loadPendings () {
	logs.Log.Info("Loading Pending TXs")
	PendingsLocker.Lock()
	db.Locker.Lock()
	_ = db.DB.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_PENDING_HASH}
		var err error
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			value, err := item.Value()
			if err != nil {
				continue
			}
			var hash []byte
			buf := bytes.NewBuffer(value)
			dec := gob.NewDecoder(buf)
			err = dec.Decode(&hash)
			if err == nil {
				pendings[convert.BytesToTrytes(hash)[:81]] = time.Now().UnixNano()
			}
		}
		return err
	})
	db.Locker.Unlock()
	PendingsLocker.Unlock()
	logs.Log.Info("Pending TXs Loaded")
}

func periodicRequest() {
	ticker := time.NewTicker(pingInterval)
	for range ticker.C {
		request(nil, "", false)
	}
}

func periodicTipRequest() {
	ticker := time.NewTicker(tipPingInterval)
	for range ticker.C {
		request(nil, "", true)
	}
}

func requestIfMissing (hash []byte, addr string, txn *badger.Txn) (has bool, err error) {
	tx := txn
	has = true
	if txn == nil {
		tx = db.DB.NewTransaction(true)
		defer func () error {
			return tx.Commit(func(e error) {})
		}()
	}
	if !db.Has(db.GetByteKey(hash, db.KEY_HASH), tx) { //&& !db.Has(db.GetByteKey(hash, db.KEY_PENDING), tx) {
		err2 := db.Put(db.GetByteKey(hash, db.KEY_PENDING_HASH), hash, nil, nil)

		if err2 != nil {
			logs.Log.Error("Failed saving new TX request", err2)
			return has, err2
		}
		PendingsLocker.Lock()
		pendings[convert.BytesToTrytes(hash)] = time.Now().Unix()
		PendingsLocker.Unlock()

		request(hash, addr,false)

		has = false
	}
	return has, nil
}

func request (hash []byte, addr string, tip bool) {
	var resp []byte
	var ok = false
	var queue *RequestQueue

	if len(addr) > 0 {
		queue, ok = requestReplyQueues[addr]
	}

	if ok {
		var request *Request = nil
		select {
		case msg := <- *queue:
			request = msg
		default:
			request = request
		}
		if request != nil {
			if !request.Tip {
				t, err := db.GetBytes(db.GetByteKey(request.Requested, db.KEY_BYTES), nil)
				if err == nil {
					resp = t
				}
			}
			if !request.Tip && resp == nil {
				// If I do not have this TX, request from somewhere else?
				// TODO: (OPT) not always. Have a random drop ratio as in IRI.
				// requestIfMissing(msg.Requested, "", nil)
			}
		}
	}

	msg := getMessage(nil, hash, tip, nil)
	data := append((*msg.Bytes)[:1604], (*msg.Requested)[:46]...)
	srv.Outgoing <- &server.Message{addr, data}
}

func oldestPending () (string, int64) {
	var max int64 = 0
	var result string
	PendingsLocker.Lock()
	defer PendingsLocker.Unlock()
	for hash, timestamp := range pendings {
		current := time.Now().Sub(time.Unix(0, timestamp)).Nanoseconds()
		if current > max {
			result = hash
			max = current
		}
	}

	return result, max
}