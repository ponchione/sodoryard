package tool

import "testing"

func TestBoundedPositiveInt(t *testing.T) {
	large := 5000
	negative := -1
	small := 7

	if got := boundedPositiveInt(nil, 10, 100); got != 10 {
		t.Fatalf("nil value = %d, want fallback", got)
	}
	if got := boundedPositiveInt(&negative, 10, 100); got != 10 {
		t.Fatalf("negative value = %d, want fallback", got)
	}
	if got := boundedPositiveInt(&small, 10, 100); got != 7 {
		t.Fatalf("small value = %d, want requested", got)
	}
	if got := boundedPositiveInt(&large, 10, 100); got != 100 {
		t.Fatalf("large value = %d, want cap", got)
	}
}

func TestBoundedNonNegativeInt(t *testing.T) {
	large := 5000
	zero := 0
	small := 7

	if got := boundedNonNegativeInt(nil, 2, 20); got != 2 {
		t.Fatalf("nil value = %d, want fallback", got)
	}
	if got := boundedNonNegativeInt(&zero, 2, 20); got != 0 {
		t.Fatalf("zero value = %d, want zero", got)
	}
	if got := boundedNonNegativeInt(&small, 2, 20); got != 7 {
		t.Fatalf("small value = %d, want requested", got)
	}
	if got := boundedNonNegativeInt(&large, 2, 20); got != 20 {
		t.Fatalf("large value = %d, want cap", got)
	}
}
