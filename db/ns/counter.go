package ns

type Counter interface {
	CountPrefix([]byte) int
}

func Count(c Counter, namespace byte) int {
	return c.CountPrefix([]byte{namespace})
}
