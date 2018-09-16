package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
)

func init() {
	addAPICall("findTransactions", findTransactions, mainAPICalls)
}

func findTransactions(request Request, c *gin.Context, t time.Time) {
	var hashes = []string{}
	for _, address := range request.Addresses {
		if !convert.IsTrytes(address, 81) {
			replyError("Wrong address trytes", c)
			return
		}
		hashes = append(hashes, findAddresses(convert.TrytesToBytes(address)[:49], len(request.Addresses) == 1)...)
	}
	for _, bundle := range request.Bundles {
		if !convert.IsTrytes(bundle, 81) {
			replyError("Wrong bundle trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(bundle)[:49], ns.NamespaceBundle)...)
	}
	for _, approvee := range request.Approvees {
		if !convert.IsTrytes(approvee, 81) {
			replyError("Wrong approvee trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(approvee)[:49], ns.NamespaceApprovee)...)
	}
	for _, tag := range request.Tags {
		if !convert.IsTrytes(tag, 81) {
			replyError("Wrong tag trytes", c)
			return
		}
		hashes = append(hashes, find(convert.TrytesToBytes(tag)[:16], ns.NamespaceTag)...)
	}
	c.JSON(http.StatusOK, gin.H{
		"hashes":   hashes,
		"duration": getDuration(t),
	})
}

func findAddresses(trits []byte, single bool) []string {
	hashes := find(trits, ns.NamespaceAddress)
	if single && len(hashes) == 0 {
		// Workaround for IOTA wallet support. Fake transactions for positive addresses:
		balance, err := coding.GetInt64(db.Singleton, ns.AddressKey(trits, ns.NamespaceBalance))
		if err == nil && balance > 0 {
			return []string{dummyHash}
		}
	}
	return hashes
}

func find(trits []byte, prefix byte) []string {
	var response = []string{}
	db.Singleton.View(func(dbTx db.Transaction) error {
		prefix := ns.HashKey(trits, prefix)
		return dbTx.ForPrefix(prefix, false, func(key, _ []byte) (bool, error) {
			key = ns.Key(key[16:], ns.NamespaceHash)
			hash, err := dbTx.GetBytes(key)
			if err == nil {
				response = append(response, convert.BytesToTrytes(hash)[:81])
			}
			return true, nil
		})
	})
	return response
}
