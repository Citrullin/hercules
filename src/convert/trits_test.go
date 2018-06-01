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
	expected := int64(33792688625192)
	result := TritsToInt(trits)
	if !reflect.DeepEqual(expected, result) {
		t.Error("Trits int converion wrong!", expected, result)
	}
}

func TestIntToTrits(t *testing.T) {
	// 1523294944
	// [0 1 -1 1 0 -1 1 -1 0 -1 -1 1 1 0 0 0 -1 0 -1 0 -1 -1 1 1 -1 -1 1]
	trits := []int{0, 1, -1, 1, 0, -1, 1, -1, 0, -1, -1, 1, 1, 0, 0, 0, -1, 0, -1, 0, -1, -1, 1, 1, -1, -1, 1}
	expected := TritsToInt(trits)
	result := IntToTrits(expected, len(trits))
	if !reflect.DeepEqual(trits, result) {
		t.Error("Int trits converion wrong!", expected, result)
	}
}

func TestConversions(t *testing.T) {
	word := "ANSDJDAAODSA999DASDW"
	result := TritsToTrytes(BytesToTrits(TritsToBytes(TrytesToTrits(word))))
	if !reflect.DeepEqual(word, result) {
		t.Error("Wrong conversions!", result)
	}
}
