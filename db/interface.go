package db

import (
	"time"

	"github.com/spf13/viper"
)

var implementations = map[string]Constructor{}

type Constructor func(*viper.Viper) (Interface, error)

type Interface interface {
	PutBytes([]byte, []byte, *time.Duration) error
	Put([]byte, interface{}, *time.Duration) error
	GetBytes([]byte) ([]byte, error)
	GetInt64([]byte) (int64, error)
	Get([]byte, interface{}) error
	Has([]byte) bool
	CountPrefix([]byte) int
	Remove([]byte) error
	RemovePrefix([]byte) error
	RemoveKeysFromCategoryBefore(byte, int64) int
	IncrementBy([]byte, int64, bool) (int64, error)
}
