package api

import (
	"net/http"
	"time"

	"../convert"
	"../db"
	"github.com/dgraph-io/badger"
	"github.com/gin-gonic/gin"
)

func init() {
	addAPICall("findTransactions", findTransactions)
}

func findTransactions(request Request, c *gin.Context, t time.Time) {
	var hashes = []string{}
	for _, address := range request.Addresses {
		if !convert.IsTrytes(address, 81) {
			ReplyError("Wrong address trytes", c)
			return
		}
		hashes = append(hashes, findAddresses(convert.TrytesToBytes(address)[:49], len(request.Addresses) == 1)...)
	}
	for _, bundle := range request.Bundles {
		if !convert.IsTrytes(bundle, 81) {
			ReplyError("Wrong bundle trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(bundle)[:49], db.KEY_BUNDLE)...)
	}
	for _, approvee := range request.Approvees {
		if !convert.IsTrytes(approvee, 81) {
			ReplyError("Wrong approvee trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(approvee)[:49], db.KEY_APPROVEE)...)
	}
	for _, tag := range request.Tags {
		if !convert.IsTrytes(tag, 81) {
			ReplyError("Wrong tag trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(tag)[:16], db.KEY_APPROVEE)...)
	}
	c.JSON(http.StatusOK, gin.H{
		"hashes":   hashes,
		"duration": getDuration(t),
	})
}

func findAddresses(trits []byte, single bool) []string {
	hashes := find(trits, db.KEY_ADDRESS)
	if single && len(hashes) == 0 {
		// Workaround for IOTA wallet support. Fake transactions for positive addresses:
		balance, err := db.GetInt64(db.GetAddressKey(trits, db.KEY_BALANCE), nil)
		if err == nil && balance > 0 {
			return []string{dummyHash}
		}
	}
	return hashes
}

func find(trits []byte, prefix byte) []string {
	var response = []string{}
	_ = db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := db.GetByteKey(trits, prefix)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := db.AsKey(it.Item().Key()[16:], db.KEY_HASH)
			hash, err := db.GetBytes(key, txn)
			if err == nil {
				response = append(response, convert.BytesToTrytes(hash)[:81])
			}
		}
		return nil
	})
	return response
}
