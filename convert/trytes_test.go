package convert

import "testing"
import (
	"reflect"
)

func TestTrytesToTrits(t *testing.T) {
	expected := []int{-1, 0, 1, -1, -1, 1, 0, 1, 1, 0, 1, 1, 0, -1, -1, -1, -1, 0, 0, -1, -1, 0, 0, -1, 0, 1, 1, 1, 1, 0}
	trits := TrytesToTrits("HELLOWORLD")
	if !reflect.DeepEqual(trits, expected) {
		t.Error("Trits wrong!")
	}
}

func TestTrytesToTrits2(t *testing.T) {
	trits := TrytesToTrits("HELLOWORLD11sd")
	if trits != nil {
		t.Error("Trits wrong!")
	}
}

func TestTrytesToTrits3(t *testing.T) {
	expected := "99CARRIOTA9FIELD9DONATION99"
	trits := TrytesToTrits("99CARRIOTA9FIELD9DONATION99")
	result := TritsToTrytes(trits)
	if result != expected {
		t.Error("Trits wrong!")
	}
}

func TestTritsToTrytes(t *testing.T) {
	trits := []int{-1, 0, 1, -1, -1, 1, 0, 1, 1, 0, 1, 1, 0, -1, -1, -1, -1, 0, 0, -1, -1, 0, 0, -1, 0, 1, 1, 1, 1, 0}
	expected := TritsToTrytes(trits)
	if !reflect.DeepEqual(expected, "HELLOWORLD") {
		t.Error("Trits wrong!", expected)
	}
}

func TestTrytesToBytes(t *testing.T) {
	expected := "99CARRIOTA9FIELD9DONATION99"
	b := TrytesToBytes("99CARRIOTA9FIELD9DONATION99")[:16]
	result := BytesToTrytes(b)
	if result != expected {
		t.Error("Bytes wrong!")
	}
}
