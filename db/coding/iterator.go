package coding

import (
	"bytes"
	"encoding/gob"

	"../../logs"
)

type Iterator interface {
	ForPrefix([]byte, bool, func([]byte, []byte) (bool, error)) error
}

func ForPrefixInt64(i Iterator, prefix []byte, skipOnError bool, fn func([]byte, int64) (bool, error)) error {
	return i.ForPrefix(prefix, true, func(key, value []byte) (bool, error) {
		var int64Value = int64(0)
		if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&int64Value); err != nil {
			if skipOnError {
				logs.Log.Error("couldn't load key value", key, err)
				return true, nil
			}
			return false, err
		}
		return fn(key, int64Value)
	})
}
