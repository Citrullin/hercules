package api

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gitlab.com/semkodev/hercules/config"
	"gitlab.com/semkodev/hercules/logs"

	"github.com/gin-gonic/gin"
	"github.com/iotaledger/giota"
	"github.com/muxxer/diverdriver"
)

const (
	// not defined in giota library
	MaxTimestampValue = 3812798742493 // int64(3^27 - 1) / 2
)

var (
	powLock                 = &sync.Mutex{}
	maxMinWeightMagnitude   = 0
	maxTransactions         = 0
	useDiverDriver          = false
	diverClient             *diverdriver.DiverClient
	interruptAttachToTangle = false
	powInitialized          = false
	powFunc                 giota.PowFunc
	powType                 string
	powVersion              string
	serverVersion           string
)

func init() {
	addAPICall("attachToTangle", attachToTangle, mainAPICalls)
	addAPICall("interruptAttachingToTangle", interruptAttachingToTangle, mainAPICalls)
}

func startAttach() {
	maxMinWeightMagnitude = config.AppConfig.GetInt("api.pow.maxMinWeightMagnitude")
	maxTransactions = config.AppConfig.GetInt("api.pow.maxTransactions")
	useDiverDriver = config.AppConfig.GetBool("api.pow.useDiverDriver")

	logs.Log.Debug("maxMinWeightMagnitude:", maxMinWeightMagnitude)
	logs.Log.Debug("maxTransactions:", maxTransactions)
	logs.Log.Debug("useDiverDriver:", useDiverDriver)

	if useDiverDriver {
		diverClient = &diverdriver.DiverClient{DiverDriverPath: config.AppConfig.GetString("api.pow.diverDriverPath"), WriteTimeOutMs: 500, ReadTimeOutMs: 120000}
	}
}

// TODO: maybe the trytes/trits/runes conversions should be ported to the version in gIOTA library
// Hercules still uses home-brew conversions and types
func IsValidPoW(hash giota.Trits, mwm int) bool {
	for i := len(hash) - mwm; i < len(hash); i++ {
		if hash[i] != 0 {
			return false
		}
	}
	return true
}

func toRunesCheckTrytes(s string, length int) ([]rune, error) {
	if len(s) != length {
		return []rune{}, errors.New("invalid length")
	}
	if _, err := giota.ToTrytes(s); err != nil {
		return []rune{}, err
	}
	return []rune(string(s)), nil
}

func toRunes(trytes giota.Trytes) []rune {
	return []rune(string(trytes))
}

// interrupts not PoW itself (no PoW of gIOTA supports interrupts) but stops
// attachToTangle after the last transaction PoWed
func interruptAttachingToTangle(request Request, c *gin.Context, ts time.Time) {
	interruptAttachToTangle = true
	c.JSON(http.StatusOK, gin.H{})
}

func getTimestampMilliseconds() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// attachToTangle
// do everything with trytes and save time by not convertig to trits and back
// all constants have to be divided by 3
func attachToTangle(request Request, c *gin.Context, ts time.Time) {
	// only one attatchToTangle allowed in parallel
	powLock.Lock()
	defer powLock.Unlock()

	interruptAttachToTangle = false

	var returnTrytes []string

	trunkTransaction, err := toRunesCheckTrytes(request.TrunkTransaction, giota.TrunkTransactionTrinarySize/3)
	if err != nil {
		replyError("Invalid trunkTransaction-Trytes", c)
		return
	}

	branchTransaction, err := toRunesCheckTrytes(request.BranchTransaction, giota.BranchTransactionTrinarySize/3)
	if err != nil {
		replyError("Invalid branchTransaction-Trytes", c)
		return
	}

	minWeightMagnitude := request.MinWeightMagnitude

	// restrict minWeightMagnitude
	if minWeightMagnitude > maxMinWeightMagnitude {
		replyError(fmt.Sprintf("MinWeightMagnitude too high. MWM: %v Allowed: %v", minWeightMagnitude, maxMinWeightMagnitude), c)
		return
	}

	trytes := request.Trytes

	// limit number of transactions in a bundle
	if len(trytes) > maxTransactions {
		replyError("Too many transactions", c)
		return
	}
	returnTrytes = make([]string, len(trytes))
	inputRunes := make([][]rune, len(trytes))

	// validate input trytes before doing PoW
	for idx, tryte := range trytes {
		if runes, err := toRunesCheckTrytes(tryte, giota.TransactionTrinarySize/3); err != nil {
			replyError("Error in Tryte input", c)
			return
		} else {
			inputRunes[idx] = runes
		}
	}

	var prevTransaction []rune

	if !powInitialized {
		if useDiverDriver {
			serverVersion, powType, powVersion, err = diverClient.GetPowInfo()
			if err != nil {
				replyError(err.Error(), c)
				return
			}

			powFunc = diverClient.PowFunc
		} else {
			powType, powFunc = giota.GetBestPoW()
		}
		powInitialized = true
	}

	if useDiverDriver {
		logs.Log.Debugf("[PoW] Using diverDriver version \"%v\"", serverVersion)
		logs.Log.Debugf("[PoW] Best method \"%v\"", powType)
		if powVersion != "" {
			logs.Log.Debugf("[PoW] Version \"%v\"", powVersion)
		}
	} else {
		logs.Log.Debugf("[PoW] Best method \"%v\"", powType)
	}

	// do pow
	for idx, runes := range inputRunes {
		var invalid = true
		for invalid {
			if interruptAttachToTangle {
				replyError("attatchToTangle interrupted", c)
				return
			}
			timestamp := getTimestampMilliseconds()

			// branch and trunk
			tmp := prevTransaction
			if len(prevTransaction) == 0 {
				tmp = trunkTransaction
			}
			copy(runes[giota.TrunkTransactionTrinaryOffset/3:], tmp[:giota.TrunkTransactionTrinarySize/3])

			tmp = trunkTransaction
			if len(prevTransaction) == 0 {
				tmp = branchTransaction
			}
			copy(runes[giota.BranchTransactionTrinaryOffset/3:], tmp[:giota.BranchTransactionTrinarySize/3])

			// attachment fields: tag and timestamps
			// tag - copy the obsolete tag to the attachment tag field only if tag isn't set.
			if string(runes[giota.TagTrinaryOffset/3:(giota.TagTrinaryOffset+giota.TagTrinarySize)/3]) == "999999999999999999999999999" {
				copy(runes[giota.TagTrinaryOffset/3:], runes[giota.ObsoleteTagTrinaryOffset/3:(giota.ObsoleteTagTrinaryOffset+giota.ObsoleteTagTrinarySize)/3])
			}

			runesTimeStamp := toRunes(giota.Int2Trits(timestamp, giota.AttachmentTimestampTrinarySize).Trytes())
			runesTimeStampLowerBoundary := toRunes(giota.Int2Trits(0, giota.AttachmentTimestampLowerBoundTrinarySize).Trytes())
			runesTimeStampUpperBoundary := toRunes(giota.Int2Trits(MaxTimestampValue, giota.AttachmentTimestampUpperBoundTrinarySize).Trytes())

			copy(runes[giota.AttachmentTimestampTrinaryOffset/3:], runesTimeStamp[:giota.AttachmentTimestampTrinarySize/3])
			copy(runes[giota.AttachmentTimestampLowerBoundTrinaryOffset/3:], runesTimeStampLowerBoundary[:giota.AttachmentTimestampLowerBoundTrinarySize/3])
			copy(runes[giota.AttachmentTimestampUpperBoundTrinaryOffset/3:], runesTimeStampUpperBoundary[:giota.AttachmentTimestampUpperBoundTrinarySize/3])

			startTime := time.Now()
			nonceTrytes, err := powFunc(giota.Trytes(runes), minWeightMagnitude)
			if err != nil || len(nonceTrytes) != giota.NonceTrinarySize/3 {
				replyError(fmt.Sprintf("PoW failed! %v", err.Error()), c)
				return
			}
			elapsedTime := time.Now().Sub(startTime)
			logs.Log.Debug("[PoW] Needed", elapsedTime)

			// copy nonce to runes
			copy(runes[giota.NonceTrinaryOffset/3:], toRunes(nonceTrytes)[:giota.NonceTrinarySize/3])

			verifyTrytes, err := giota.ToTrytes(string(runes))
			if err != nil {
				replyError("Trytes got corrupted", c)
				return
			}

			// validate PoW - throws exception if invalid
			hash := verifyTrytes.Hash()
			if !IsValidPoW(hash.Trits(), minWeightMagnitude) {
				logs.Log.Debug("Nonce verification failed. Retrying...", hash)
				continue
			}

			invalid = false

			logs.Log.Debug("[PoW] Verified!")

			returnTrytes[idx] = string(runes)

			prevTransaction = toRunes(hash)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"trytes":   returnTrytes,
		"duration": getDuration(ts),
	})
}
