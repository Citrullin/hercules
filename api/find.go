package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"../convert"
	"../db"
	"../db/coding"
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
		hashes = append(hashes, find(convert.TrytesToBytes(tag)[:16], db.KEY_TAG)...)
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
		balance, err := coding.GetInt64(db.Singleton, db.GetAddressKey(trits, db.KEY_BALANCE))
		if err == nil && balance > 0 {
			return []string{dummyHash}
		}
	}
	return hashes
}

func find(trits []byte, prefix byte) []string {
	var response = []string{}
	db.Singleton.View(func(tx db.Transaction) error {
		prefix := db.GetByteKey(trits, prefix)
		return tx.ForPrefix(prefix, true, func(key, value []byte) (bool, error) {
			key = db.AsKey(key[16:], db.KEY_HASH)
			hash, err := coding.GetBytes(tx, key)
			if err == nil {
				response = append(response, convert.BytesToTrytes(hash)[:81])
			}
			return true, nil
		})
	})
	return response
}
