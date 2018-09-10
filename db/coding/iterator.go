package coding

import (
	"encoding/binary"
	"errors"

	"../../logs"
)

type Iterator interface {
	ForPrefix([]byte, bool, func([]byte, []byte) (bool, error)) error
}

type RemoveIterator interface {
	Iterator
	Remove([]byte) error
}

func HasKeyInCategoryWithInt64LowerEqual(i Iterator, keyCategory byte, threshold int64) bool {
	result := false
	ForPrefixInt64(i, []byte{keyCategory}, true, func(_ []byte, value int64) (bool, error) {
		if value > 0 && value <= threshold {
			result = true
			return false, nil
		}
		return true, nil
	})
	return result
}

func RemoveKeysInCategoryWithInt64LowerEqual(i RemoveIterator, keyCategory byte, threshold int64) int {
	var keys [][]byte
	ForPrefixInt64(i, []byte{keyCategory}, true, func(key []byte, value int64) (bool, error) {
		if value < threshold {
			keys = append(keys, key)
		}
		return true, nil
	})

	for _, key := range keys {
		i.Remove(key)
	}

	return len(keys)
}

func SumInt64InCategory(i Iterator, keyCategory byte) int64 {
	sum := int64(0)
	ForPrefixInt64(i, []byte{keyCategory}, false, func(_ []byte, value int64) (bool, error) {
		sum += value
		return true, nil
	})
	return sum
}

func ForPrefixInt64(i Iterator, prefix []byte, skipOnError bool, fn func([]byte, int64) (bool, error)) error {
	return i.ForPrefix(prefix, true, func(key, value []byte) (bool, error) {
		int64Value, n := binary.Varint(value)
		if n <= 0 {
			if skipOnError {
				logs.Log.Error("value larger than 64 bits (overflow)")
				return true, nil
			}
			return false, errors.New("value larger than 64 bits (overflow)")
		}

		return fn(key, int64Value)
	})
}

func ForPrefixInt(i Iterator, prefix []byte, skipOnError bool, fn func([]byte, int) (bool, error)) error {
	return i.ForPrefix(prefix, true, func(key, value []byte) (bool, error) {
		int64Value, n := binary.Varint(value)
		if n <= 0 {
			if skipOnError {
				logs.Log.Error("value larger than 64 bits (overflow)")
				return true, nil
			}
			return false, errors.New("value larger than 64 bits (overflow)")
		}

		return fn(key, int(int64Value))
	})
}
