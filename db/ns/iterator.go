package ns

type Iterator interface {
	ForPrefix([]byte, bool, func([]byte, []byte) (bool, error)) error
}

func ForNamespace(i Iterator, namespace byte, fetchValues bool, fn func([]byte, []byte) (bool, error)) error {
	return i.ForPrefix(Prefix(namespace), fetchValues, fn)
}
