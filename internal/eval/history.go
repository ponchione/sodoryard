package eval

import "time"

type HistoryEntry struct {
	RecordedAt     time.Time `json:"recorded_at"`
	Suite          string    `json:"suite"`
	Status         string    `json:"status"`
	Score          float64   `json:"score"`
	Totals         Totals    `json:"totals"`
	BaselinePath   string    `json:"baseline_path,omitempty"`
	BaselineStatus string    `json:"baseline_status,omitempty"`
	BaselineDiffs  int       `json:"baseline_diffs,omitempty"`
}

func NewHistoryEntry(report Report, recordedAt time.Time) HistoryEntry {
	if recordedAt.IsZero() {
		recordedAt = time.Now()
	}
	entry := HistoryEntry{
		RecordedAt: recordedAt.UTC(),
		Suite:      report.Suite,
		Status:     report.Status,
		Score:      report.Score,
		Totals:     report.Totals,
	}
	if report.Baseline != nil {
		entry.BaselinePath = report.Baseline.Path
		entry.BaselineStatus = report.Baseline.Status
		entry.BaselineDiffs = len(report.Baseline.Diffs)
	}
	return entry
}
