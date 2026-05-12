package tracking

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	sid "github.com/ponchione/sodoryard/internal/id"
	"github.com/ponchione/sodoryard/internal/projectmemory"
)

type ProjectMemorySubCallStore struct {
	recorder projectmemory.SubCallRecorder
	newID    func() string
	now      func() time.Time
}

var _ SubCallStore = (*ProjectMemorySubCallStore)(nil)

func NewProjectMemorySubCallStore(recorder projectmemory.SubCallRecorder) *ProjectMemorySubCallStore {
	return &ProjectMemorySubCallStore{
		recorder: recorder,
		newID:    sid.New,
		now:      time.Now,
	}
}

func (s *ProjectMemorySubCallStore) InsertSubCall(ctx context.Context, params InsertSubCallParams) error {
	if s == nil || s.recorder == nil {
		return fmt.Errorf("project memory sub-call recorder is nil")
	}
	completedAtUS := parseCreatedAtUS(params.CreatedAt, s.now)
	startedAtUS := completedAtUS
	if params.LatencyMs > 0 {
		latencyUS := uint64(params.LatencyMs) * 1000
		if completedAtUS > latencyUS {
			startedAtUS = completedAtUS - latencyUS
		}
	}
	status := "error"
	if params.Success != 0 {
		status = "success"
	}
	args := projectmemory.RecordSubCallArgs{
		ID:                  s.newID(),
		ConversationID:      stringValue(params.ConversationID),
		MessageID:           messageIDString(params.MessageID),
		TurnNumber:          uint32PtrValue(params.TurnNumber),
		Iteration:           uint32PtrValue(params.Iteration),
		Provider:            params.Provider,
		Model:               params.Model,
		Purpose:             params.Purpose,
		Status:              status,
		StartedAtUS:         startedAtUS,
		CompletedAtUS:       completedAtUS,
		TokensIn:            nonNegativeUint64(params.TokensIn),
		TokensOut:           nonNegativeUint64(params.TokensOut),
		CacheReadTokens:     nonNegativeUint64(params.CacheReadTokens),
		CacheCreationTokens: nonNegativeUint64(params.CacheCreationTokens),
		LatencyMs:           nonNegativeUint64(params.LatencyMs),
		Error:               stringValue(params.ErrorMessage),
		MetadataJSON:        "{}",
	}
	return s.recorder.RecordSubCall(ctx, args)
}

func parseCreatedAtUS(value string, now func() time.Time) uint64 {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return uint64(parsed.UTC().UnixMicro())
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return uint64(parsed.UTC().UnixMicro())
		}
	}
	if now == nil {
		now = time.Now
	}
	return uint64(now().UTC().UnixMicro())
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func messageIDString(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func uint32PtrValue(value *int) uint32 {
	if value == nil || *value <= 0 {
		return 0
	}
	return uint32(*value)
}

func nonNegativeUint64[T ~int | ~int64](value T) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}
