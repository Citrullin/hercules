package api

// complaints or suggestions pls to pmaxuw on discord

import (
    "net/http"
    "sync"
    "time"
    "errors"

    "../logs"

//    "github.com/spf13/viper"

    "github.com/spf13/viper"
    "github.com/gin-gonic/gin"
    giota "github.com/iotaledger/giota"
    pidiver "github.com/shufps/pidiver"
)

const (
    // not defined in giota library
    MaxTimestampValue = 3812798742493 //int64(3^27 - 1) / 2
     
    // wth didn't they export this ...?
    TransactionTrinarySize = giota.SignatureMessageFragmentTrinarySize + giota.AddressTrinarySize +
        giota.ValueTrinarySize + giota.ObsoleteTagTrinarySize + giota.TimestampTrinarySize +
        giota.CurrentIndexTrinarySize + giota.LastIndexTrinarySize + giota.BundleTrinarySize +
        giota.TrunkTransactionTrinarySize + giota.BranchTransactionTrinarySize +
        giota.TagTrinarySize + giota.AttachmentTimestampTrinarySize +
        giota.AttachmentTimestampLowerBoundTrinarySize + giota.AttachmentTimestampUpperBoundTrinarySize +
        giota.NonceTrinarySize     
)

var mutex = &sync.Mutex{}
var maxMinWeightMagnitude = 14
var maxTransactions = 5
var usePiDiver bool = false

func init() {
    addStartModule(startAttach)
    
    addAPICall("attachToTangle", attachToTangle)
    addAPICall("interruptAttachingToTangle", interruptAttachingToTangle)
}

func startAttach(apiConfig *viper.Viper) {
    maxMinWeightMagnitude = config.GetInt("api.pow.maxMinWeightMagnitude")
    maxTransactions = config.GetInt("api.pow.maxTransactions")
    usePiDiver = config.GetBool("api.pow.usePiDiver")
    
    logs.Log.Info("maxMinWeightMagnitude:", maxMinWeightMagnitude)
    logs.Log.Info("maxTransactions:", maxTransactions)
    logs.Log.Info("usePiDiver:", usePiDiver)
    
    if usePiDiver  {
		err := pidiver.InitPiDiver()
		if err != nil {
			logs.Log.Warning("PiDiver cannot be used. Error while initialization.")
            usePiDiver = false
		}
    }
    
}

func IsValidPoW(hash giota.Trits, mwm int) bool {
	for i := len(hash) - mwm; i < len(hash); i++ {
		if hash[i] != 0 {
			return false
		}
	}
	return true
}

func isValidHash(hash []rune, length int) bool {
    if len(hash) != length {
        return false
    }
    
    if _, err := giota.ToTrytes(string(hash)); err != nil {
        return false
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


func toRunes(t giota.Trytes) []rune {
    return []rune(string(t))
}


// No PoW method of giota supports interrupt!
// If someone knows a good idea (perhaps running a thread and cancelling it)
// let me know
func interruptAttachingToTangle(request Request, c *gin.Context, t time.Time) {
    c.JSON(http.StatusOK, gin.H{
    })    
}

func getTimestamp() int64 {
    return time.Now().UnixNano() / (int64(time.Millisecond)/int64(time.Nanosecond))   // time.Nanosecond should always be 1 ... but if not ...^^
}

// attachToTangle
// do everything with trytes and save time by not convertig to trits and back
// all constants have to be divided by 3
// TODO perhaps both methods only should be callable from a trusted network
func attachToTangle(request Request, c *gin.Context, t time.Time) {
    // only one attatchToTangle allowed in parallel
    mutex.Lock()
    defer mutex.Unlock()
    
    var returnTrytes []string
    
    trunkTransaction, err := toRunesCheckTrytes(request.TrunkTransaction, giota.TrunkTransactionTrinarySize/3)
    if err != nil {
        ReplyError("Invalid trunkTransaction-Trytes", c)
        return
    }
    
    branchTransaction, err := toRunesCheckTrytes(request.BranchTransaction, giota.BranchTransactionTrinarySize/3)
    if err != nil {
        ReplyError("Invalid branchTransaction-Trytes", c)
        return
    }
    
    minWeightMagnitude := request.MinWeightMagnitude
    
    // TODO prevent DOS attacks ... What is the allowed range? 
    // IRI says CURL_HASH_LENGTH but that's too high ...
    // default for main-net is 14
    if minWeightMagnitude > maxMinWeightMagnitude {
        ReplyError("MinWeightMagnitude too high", c)
        return
    }
    
    trytes := request.Trytes
    
    // TODO how many transactions are allowed? what says IRI?
    // default for non-zero-value bundle with 5TX
    if len(trytes) > maxTransactions {
        ReplyError("Too many transactions", c)
        return
    }
    returnTrytes = make([]string, len(trytes))
    inputRunes := make([][]rune, len(trytes))

    // validate input trytes before doing PoW
    for idx, tryte := range trytes {
        if runes, err := toRunesCheckTrytes(tryte, TransactionTrinarySize/3); err != nil {
            ReplyError("Error in Tryte input", c)
            return
        } else {
            inputRunes[idx] = runes;
        }
    }

    var prevTransaction []rune
    
    for idx, runes := range inputRunes {
        timestamp := getTimestamp()
        //branch and trunk
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
        
        //attachment fields: tag and timestamps
        //tag - copy the obsolete tag to the attachment tag field only if tag isn't set.
        if string(runes[giota.TagTrinaryOffset/3:(giota.TagTrinaryOffset+giota.TagTrinarySize)/3]) == "999999999999999999999999999" {
            copy(runes[giota.TagTrinarySize/3:], runes[giota.ObsoleteTagTrinaryOffset/3:(giota.ObsoleteTagTrinaryOffset+giota.ObsoleteTagTrinarySize)/3])
        }
        
        runesTimeStamp := toRunes(giota.Int2Trits(timestamp, giota.AttachmentTimestampTrinarySize).Trytes())
        runesTimeStampLowerBoundary := toRunes(giota.Int2Trits(0, giota.AttachmentTimestampLowerBoundTrinarySize).Trytes())
        runesTimeStampUpperBoundary := toRunes(giota.Int2Trits(MaxTimestampValue, giota.AttachmentTimestampUpperBoundTrinarySize).Trytes())
        
        copy(runes[giota.AttachmentTimestampTrinaryOffset/3:], runesTimeStamp[:giota.AttachmentTimestampTrinarySize/3])
        copy(runes[giota.AttachmentTimestampLowerBoundTrinaryOffset/3:], runesTimeStampLowerBoundary[:giota.AttachmentTimestampLowerBoundTrinarySize/3])
        copy(runes[giota.AttachmentTimestampUpperBoundTrinaryOffset/3:], runesTimeStampUpperBoundary[:giota.AttachmentTimestampUpperBoundTrinarySize/3])

        var powFunc giota.PowFunc
        var pow string

        // do pow        
        if usePiDiver {
            logs.Log.Info("[PoW] Using PiDiver")
            powFunc = pidiver.PowPiDiver
            pow = "FPGA (PiDiver)"
        } else {
            pow, powFunc = giota.GetBestPoW()
        }
        logs.Log.Info("[PoW] Best method", pow)

        startTime := time.Now()
        nonceTrytes, err := powFunc(giota.Trytes(runes), minWeightMagnitude)   
        if err != nil || len(nonceTrytes) != giota.NonceTrinarySize/3 {
            ReplyError("PoW failed!", c)
            return
        }
        elapsedTime := time.Now().Sub(startTime)
        logs.Log.Info("[PoW] Needed", elapsedTime)
        
        // copy nonce to runes
        copy(runes[giota.NonceTrinaryOffset/3:], toRunes(nonceTrytes)[:giota.NonceTrinarySize/3])
                
        verifyTrytes, err := giota.ToTrytes(string(runes))
        if err != nil {
            ReplyError("Trytes got corrupted", c)
            return
        }
        
        //validate PoW - throws exception if invalid
        hash := verifyTrytes.Hash()
        if !IsValidPoW(hash.Trits(), minWeightMagnitude) {
            ReplyError("Nonce verify failed", c)
            return
        }
        
        logs.Log.Info("[PoW] Verified!")

        returnTrytes[idx] = string(runes);

        prevTransaction = toRunes(hash);
    }    

    c.JSON(http.StatusOK, gin.H{
        "trytes":   returnTrytes,
    })
}
