package api

import (
	"net/http"
	"time"
	"bytes"
	"encoding/gob"
	"github.com/gin-gonic/gin"
	"github.com/dgraph-io/badger"
	"gitlab.com/semkodev/hercules/convert"
	"gitlab.com/semkodev/hercules/db"
	"gitlab.com/semkodev/hercules/tangle"
	"gitlab.com/semkodev/hercules/logs"
)

func getBalances (request Request, c *gin.Context, t time.Time) {
	if request.Addresses != nil {
		var balances = []int64{}
		for _, address := range request.Addresses {
			if !convert.IsTrytes(address, 81) {
				ReplyError("Wrong trytes", c)
				return
			}
			addressBytes := convert.TrytesToBytes(address)[:49]
			if addressBytes == nil {
				balances = append(balances, 0)
				continue
			}
			balance, err := db.GetInt64(db.GetAddressKey(addressBytes, db.KEY_BALANCE), nil)
			if err != nil {
				balances = append(balances, 0)
				continue
			}
			balances = append(balances, balance)
		}
		c.JSON(http.StatusOK, gin.H{
			"balances": balances,
			"duration": getDuration(t),
			"milestone": convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
			"milestoneIndex": tangle.LatestMilestone.Index,
		})
	}
}

func listAllAccounts (request Request, c *gin.Context, t time.Time) {
	var accounts = make(map[string]interface{})
	db.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte{db.KEY_BALANCE}

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()
			v, err := item.Value()
			if err == nil {
				var value int64 = 0
				buf := bytes.NewBuffer(v)
				dec := gob.NewDecoder(buf)
				err := dec.Decode(&value)
				if err == nil {
					// Do not save zero-value addresses
					if value == 0 { continue }

					accounts[convert.BytesToTrytes(key[1:])[:81]] = value
				} else {
					logs.Log.Error("Could not parse a snapshot value from database!", err)
					return err
				}
			} else {
				logs.Log.Error("Could not read a snapshot value from database!", err)
				return err
			}
		}
		return nil
	})
	c.JSON(http.StatusOK, gin.H{
		"accounts":       accounts,
		"duration":       getDuration(t),
		"milestone":      convert.BytesToTrytes(tangle.LatestMilestone.TX.Hash)[:81],
		"milestoneIndex": tangle.LatestMilestone.Index,
	})
}