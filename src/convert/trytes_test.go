package convert

import "testing"
import "reflect"

func TestTrytesToTrits(t *testing.T) {
	expected := []int{-1, 0, 1, -1, -1, 1, 0, 1, 1, 0, 1, 1, 0, -1, -1, -1, -1, 0, 0, -1, -1, 0, 0, -1, 0, 1, 1, 1, 1, 0}
	trits := TrytesToTrits("HELLOWORLD")
	if !reflect.DeepEqual(trits, expected) {
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
