package chain

import "testing"

func TestNextControlStatus(t *testing.T) {
	tests := []struct {
		name   string
		cur    string
		target string
		want   string
	}{
		{name: "pause running becomes requested", cur: "running", target: "paused", want: "pause_requested"},
		{name: "cancel running becomes requested", cur: "running", target: "cancelled", want: "cancel_requested"},
		{name: "cancel paused is immediate", cur: "paused", target: "cancelled", want: "cancelled"},
		{name: "resume pause requested returns running", cur: "pause_requested", target: "running", want: "running"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NextControlStatus(tc.cur, tc.target)
			if err != nil {
				t.Fatalf("NextControlStatus() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("NextControlStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFinalizeControlStatus(t *testing.T) {
	if got, ok := FinalizeControlStatus("pause_requested"); !ok || got != "paused" {
		t.Fatalf("FinalizeControlStatus(pause_requested) = (%q, %t), want (paused, true)", got, ok)
	}
	if got, ok := FinalizeControlStatus("cancel_requested"); !ok || got != "cancelled" {
		t.Fatalf("FinalizeControlStatus(cancel_requested) = (%q, %t), want (cancelled, true)", got, ok)
	}
	if got, ok := FinalizeControlStatus("running"); ok || got != "" {
		t.Fatalf("FinalizeControlStatus(running) = (%q, %t), want (\"\", false)", got, ok)
	}
}

func TestShouldStopScheduling(t *testing.T) {
	for _, status := range []string{"paused", "cancelled", "pause_requested", "cancel_requested"} {
		if !ShouldStopScheduling(status) {
			t.Fatalf("ShouldStopScheduling(%q) = false, want true", status)
		}
	}
	if ShouldStopScheduling("running") {
		t.Fatal("ShouldStopScheduling(running) = true, want false")
	}
}
