package chaininput

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeRoleSetTrimsAndDeduplicatesCaseInsensitive(t *testing.T) {
	got := NormalizeRoleSet([]string{" coder ", "Coder", "", "planner"})
	want := []string{"coder", "planner"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeRoleSet() = %v, want %v", got, want)
	}
}

func TestNormalizeRoleListKeepsOrderAndDuplicates(t *testing.T) {
	got := NormalizeRoleList([]string{" coder ", "coder", "", "planner"})
	want := []string{"coder", "coder", "planner"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeRoleList() = %v, want %v", got, want)
	}
}

func TestNormalizeLimitsAppliesSharedDefaults(t *testing.T) {
	got := NormalizeLimits(Limits{})
	if got.MaxSteps != DefaultMaxSteps || got.MaxResolverLoops != DefaultMaxResolverLoops || got.MaxDuration != DefaultMaxDuration || got.TokenBudget != DefaultTokenBudget {
		t.Fatalf("NormalizeLimits() = %+v, want shared defaults", got)
	}
	if got.MaxDuration != 4*time.Hour {
		t.Fatalf("DefaultMaxDuration = %s, want 4h", got.MaxDuration)
	}
}
