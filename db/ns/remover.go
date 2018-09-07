package ns

type Remover interface {
	RemovePrefix([]byte) error
}

func Remove(r Remover, namespace byte) error {
	return r.RemovePrefix(Prefix(namespace))
}
