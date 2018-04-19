package convert

import (
	"testing"
	"reflect"
)

func TestTritsToBytes(t *testing.T) {
	trits := []int{-1, 0, 1, -1, -1, 1, 0, 1, 1, 0, 1, 1, 0, -1, -1, -1, -1, 0, 0, -1, -1, 0, 0, -1, 0, 1, 1, 1, 1, 0}
	result := TritsToBytes(trits)
	expected := []byte{156, 37, 152, 171, 228, 40}
	if !reflect.DeepEqual(expected, result) {
		t.Error("Bytes wrong!", expected)
	}
}

func TestBytesToTrits(t *testing.T) {
	trits := []int{-1, 0, 1, -1, -1, 1, 0, 1, 1, 0, 1, 1, 0, -1, -1, -1, -1, 0, 0, -1, -1, 0, 0, -1, 0, 1, 1, 1, 1, 0}
	bytes := []byte{156, 37, 152, 171, 228, 40}
	result := BytesToTrits(bytes)
	if !reflect.DeepEqual(trits, result) {
		t.Error("Bytes wrong!", result)
	}
}

func TestTritsToInt(t *testing.T) {
	trits := TrytesToTrits("HELLOWORLD")
	expected := 33792688625192
	result := TritsToInt(trits)
	if !reflect.DeepEqual(expected, result) {
		t.Error("Trits int converion wrong!", expected, result)
	}
}

func TestConversions(t *testing.T) {
	word := "ANSDJDAAODSA999DASDW"
	result := TritsToTrytes(BytesToTrits(TritsToBytes(TrytesToTrits(word))))
	if !reflect.DeepEqual(word, result) {
		t.Error("Wrong conversions!", result)
	}
}
