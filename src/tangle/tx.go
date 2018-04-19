package tangle

import (
	"crypt"
	"convert"
)

type TX struct {
	Hash                          string
	SignatureMessageFragment      string
	Address                       string
	Value                         int
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
		Value:                         value(trits[6804:6837]),
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
	return convert.TritsToInt(trits)
}