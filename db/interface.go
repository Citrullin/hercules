package db

import (
	"fmt"
)

var implementations = map[string]Constructor{}

func RegisterImplementation(name string, constructor Constructor) {
	if _, ok := implementations[name]; ok {
		panic(fmt.Sprintf("database implementation with name [%s] is already registered", name))
	}
	implementations[name] = constructor
}

type Constructor func() (Interface, error)

type Manipulator interface {
	GetBytes(key []byte) ([]byte, error)
	PutBytes(key []byte, bytes []byte) error
	HasKey(key []byte) bool
	Remove(key []byte) error
	RemovePrefix(prefix []byte) error
	CountPrefix(prefix []byte) int
	ForPrefix(prefix []byte, fetchValues bool, fn func([]byte, []byte) (bool, error)) error
}

type Interface interface {
	Manipulator
	Lock()
	Unlock()
	NewTransaction(update bool) Transaction
	Update(func(Transaction) error) error
	View(func(Transaction) error) error
	Close() error
	End()
}

type Transaction interface {
	Manipulator
	Discard()
	Commit() error
}
