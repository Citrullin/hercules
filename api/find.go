package api

import (
	"time"
	"net/http"
	"github.com/gin-gonic/gin"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules.go/convert"
	"gitlab.com/semkodev/hercules.go/db"
)

func findTransactions (request Request, c *gin.Context, t time.Time) {
	var hashes = []string{}
	for _, address := range request.Addresses {
		if !convert.IsTrytes(address, 81) {
			ReplyError("Wrong address trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(address)[:49], db.KEY_ADDRESS)...)
	}
	for _, bundle := range request.Bundles {
		if !convert.IsTrytes(bundle, 81) {
			ReplyError("Wrong bundle trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(bundle)[:49], db.KEY_BUNDLE)...)
	}
	for _, approvee := range request.Approvees {
		if !convert.IsTrytes(approvee, 27) {
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
		"hashes": hashes,
		"duration": getDuration(t),
	})
}


func find (trits []byte, prefix byte) []string {
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