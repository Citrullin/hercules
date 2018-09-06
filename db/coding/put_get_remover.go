package coding

type PutGetRemover interface {
	Putter
	Getter
	Remove([]byte) error
}

func IncrementInt64By(pgr PutGetRemover, key []byte, delta int64, deleteOnZero bool) (int64, error) {
	balance, err := GetInt64(pgr, key)
	balance += delta
	if balance == 0 && deleteOnZero && err != nil {
		if err := pgr.Remove(key); err != nil {
			return balance, err
		}
	}
	err = PutInt64(pgr, key, balance)
	return balance, err
}
