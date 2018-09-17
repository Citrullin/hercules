package coding_test

import (
	"fmt"
	"runtime/debug"
	"testing"
	"time"

	"github.com/coocood/freecache"
)

func TestFreeCache(b *testing.T) {

	cacheSize := 100 * 1024

	cache := freecache.NewCache(cacheSize)
	debug.SetGCPercent(20)

	key := []byte("abc")
	val := []byte("def")

	expire := 2 // expire in 2 seconds

	cache.Set(key, val, expire)
	got, err := cache.Get(key)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(got))
	}

	fmt.Println("entry count ", cache.EntryCount())

	time.Sleep(1 * time.Second)
	fmt.Println("")
	got, err = cache.Get(key)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(got))
	}

	fmt.Println("entry count ", cache.EntryCount())

	time.Sleep(1 * time.Second)
	fmt.Println("")
	got, err = cache.Get(key)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(got))
	}

	fmt.Println("entry count ", cache.EntryCount())
}
