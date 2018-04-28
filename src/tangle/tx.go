package tangle

import (
	"crypt"
	"convert"
)

type TX struct {
	Hash                          string
	SignatureMessageFragment      string
	Address                       string
	Value                         int64
	ObsoleteTag                   string
	Timestamp                     int
	CurrentIndex                  int
	LastIndex                     int
	Bundle                        string
	TrunkTransaction              string
	BranchTransaction             string
	Tag                           string
	AttachmentTimestamp           int
	AttachmentTimestampLowerBound int
	AttachmentTimestampUpperBound int
	Nonce                         string
}

type FastTX struct {
	// Hash as bytes
	// Trunk/branch as bytes
	// Address as bytes
	// Timestamp as int
	// Value as int64
	Hash                          []byte
	Address                       []byte
	Value                         int64
	Timestamp                     int
	TrunkTransaction              []byte
	BranchTransaction             []byte
}

func TritsToFastTX (trits *[]int) *FastTX {
	return &FastTX{
		Hash: convert.TritsToBytes(crypt.Curl(*trits))[:49],
		Address: convert.TritsToBytes((*trits)[6561:6804])[:49],
		Value: value64((*trits)[6804:6837]),
		Timestamp: value((*trits)[6966:6993]),
		TrunkTransaction: convert.TritsToBytes((*trits)[7290:7533])[:49],
		BranchTransaction: convert.TritsToBytes((*trits)[7533:7776])[:49],
	}
}

func TrytesToObject (trytes string) *TX {
	if len(trytes) < 1 {
		return nil
	}

	for i := 2279; i < 2295; i++ {
		if convert.CharCodeAt(trytes, i) != '9' {
			return nil
		}
	}

	trits := convert.TrytesToTrits(trytes)
	return &TX{
		Hash:                          convert.TritsToTrytes(crypt.Curl(trits)),
		SignatureMessageFragment:      trytes[:2187],
		Address:                       trytes[2187:2268],
		Value:                         value64(trits[6804:6837]),
		ObsoleteTag:                   trytes[2295:2322],
		Timestamp:                     value(trits[6966:6993]),
		CurrentIndex:                  value(trits[6993:7020]),
		LastIndex:                     value(trits[7020:7047]),
		Bundle:                        trytes[2349:2430],
		TrunkTransaction:              trytes[2430:2511],
		BranchTransaction:             trytes[2511:2592],
		Tag:                           trytes[2592:2619],
		AttachmentTimestamp:           value(trits[7857:7884]),
		AttachmentTimestampLowerBound: value(trits[7884:7911]),
		AttachmentTimestampUpperBound: value(trits[7911:7938]),
		Nonce:                         trytes[2646:2673],
	}
}

func value (trits []int) int {
	return int(value64(trits))
}

func value64 (trits []int) int64 {
	return convert.TritsToInt(trits)
}