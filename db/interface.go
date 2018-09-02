package db

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

var implementations = map[string]Constructor{}

func RegisterImplementation(name string, constructor Constructor) {
	if _, ok := implementations[name]; ok {
		panic(fmt.Sprintf("database implementation with name [%s] is already registered", name))
	}
	implementations[name] = constructor
}

type Constructor func(*viper.Viper) (Interface, error)

type Manipulator interface {
	PutBytes([]byte, []byte, *time.Duration) error
	GetBytes([]byte) ([]byte, error)
	GetBytesRaw([]byte) ([]byte, error)
	GetInt64([]byte) (int64, error)
	GetInt([]byte) (int, error)
	GetString([]byte) (string, error)
	GetBool([]byte) (bool, error)
	Get([]byte, interface{}) error
	HasKey([]byte) bool
	HasKeysFromCategoryBefore(byte, int64) bool
	Remove([]byte) error
	RemovePrefix([]byte) error
	RemoveKeyCategory(byte) error
	RemoveKeysFromCategoryBefore(byte, int64) int
	CountKeyCategory(byte) int
	CountPrefix([]byte) int
	SumInt64FromCategory(byte) int64
	IncrementBy([]byte, int64, bool) (int64, error)
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
}

type Transaction interface {
	Manipulator
	Discard()
	Commit() error
}
