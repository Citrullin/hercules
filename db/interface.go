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
	GetBytes([]byte) ([]byte, error)
	PutBytes([]byte, []byte) error
	HasKey([]byte) bool
	Remove([]byte) error
	RemovePrefix([]byte) error
	CountPrefix([]byte) int
	ForPrefix([]byte, bool, func([]byte, []byte) (bool, error)) error
}

type Interface interface {
	Manipulator
	Lock()
	Unlock()
	NewTransaction(bool) Transaction
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
