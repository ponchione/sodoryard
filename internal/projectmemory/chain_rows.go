package projectmemory

import (
	"fmt"
	"strings"

	"github.com/ponchione/shunter/types"
)

type Chain struct {
	ID              string
	SourceSpecsJSON string
	SourceTask      string
	Status          string
	Summary         string
	CreatedAtUS     uint64
	UpdatedAtUS     uint64
	StartedAtUS     uint64
	CompletedAtUS   uint64
	MetricsJSON     string
	LimitsJSON      string
	ControlJSON     string
}

type ChainStep struct {
	ID            string
	ChainID       string
	Sequence      uint32
	Role          string
	Task          string
	TaskContext   string
	Status        string
	Verdict       string
	CreatedAtUS   uint64
	StartedAtUS   uint64
	CompletedAtUS uint64
	ReceiptPath   string
	TokensUsed    uint64
	TurnsUsed     uint64
	DurationSecs  uint64
	ExitCode      int64
	HasExitCode   bool
	Error         string
}

type ChainEvent struct {
	ID          string
	ChainID     string
	StepID      string
	Sequence    uint64
	EventType   string
	CreatedAtUS uint64
	PayloadJSON string
}

func chainRow(chain Chain) types.ProductValue {
	return types.ProductValue{
		types.NewString(chain.ID),
		types.NewString(defaultString(chain.SourceSpecsJSON, emptyJSONArray)),
		types.NewString(chain.SourceTask),
		types.NewString(defaultString(chain.Status, "running")),
		types.NewString(chain.Summary),
		types.NewUint64(chain.CreatedAtUS),
		types.NewUint64(chain.UpdatedAtUS),
		types.NewUint64(chain.StartedAtUS),
		types.NewUint64(chain.CompletedAtUS),
		types.NewString(defaultString(chain.MetricsJSON, emptyJSONObject)),
		types.NewString(defaultString(chain.LimitsJSON, emptyJSONObject)),
		types.NewString(defaultString(chain.ControlJSON, emptyJSONObject)),
	}
}

func decodeChainRow(row types.ProductValue) Chain {
	return Chain{
		ID:              row[0].AsString(),
		SourceSpecsJSON: row[1].AsString(),
		SourceTask:      row[2].AsString(),
		Status:          row[3].AsString(),
		Summary:         row[4].AsString(),
		CreatedAtUS:     row[5].AsUint64(),
		UpdatedAtUS:     row[6].AsUint64(),
		StartedAtUS:     row[7].AsUint64(),
		CompletedAtUS:   row[8].AsUint64(),
		MetricsJSON:     row[9].AsString(),
		LimitsJSON:      row[10].AsString(),
		ControlJSON:     row[11].AsString(),
	}
}

func chainStepRow(step ChainStep) types.ProductValue {
	return types.ProductValue{
		types.NewString(step.ID),
		types.NewString(step.ChainID),
		types.NewUint32(step.Sequence),
		types.NewString(step.Role),
		types.NewString(step.Task),
		types.NewString(step.TaskContext),
		types.NewString(defaultString(step.Status, "pending")),
		types.NewString(step.Verdict),
		types.NewUint64(step.CreatedAtUS),
		types.NewUint64(step.StartedAtUS),
		types.NewUint64(step.CompletedAtUS),
		types.NewString(step.ReceiptPath),
		types.NewUint64(step.TokensUsed),
		types.NewUint64(step.TurnsUsed),
		types.NewUint64(step.DurationSecs),
		types.NewInt64(step.ExitCode),
		types.NewBool(step.HasExitCode),
		types.NewString(step.Error),
	}
}

func decodeChainStepRow(row types.ProductValue) ChainStep {
	return ChainStep{
		ID:            row[0].AsString(),
		ChainID:       row[1].AsString(),
		Sequence:      row[2].AsUint32(),
		Role:          row[3].AsString(),
		Task:          row[4].AsString(),
		TaskContext:   row[5].AsString(),
		Status:        row[6].AsString(),
		Verdict:       row[7].AsString(),
		CreatedAtUS:   row[8].AsUint64(),
		StartedAtUS:   row[9].AsUint64(),
		CompletedAtUS: row[10].AsUint64(),
		ReceiptPath:   row[11].AsString(),
		TokensUsed:    row[12].AsUint64(),
		TurnsUsed:     row[13].AsUint64(),
		DurationSecs:  row[14].AsUint64(),
		ExitCode:      row[15].AsInt64(),
		HasExitCode:   row[16].AsBool(),
		Error:         row[17].AsString(),
	}
}

func chainEventRow(event ChainEvent) types.ProductValue {
	return types.ProductValue{
		types.NewString(event.ID),
		types.NewString(event.ChainID),
		types.NewString(event.StepID),
		types.NewUint64(event.Sequence),
		types.NewString(event.EventType),
		types.NewUint64(event.CreatedAtUS),
		types.NewString(defaultString(event.PayloadJSON, emptyJSONObject)),
	}
}

func decodeChainEventRow(row types.ProductValue) ChainEvent {
	return ChainEvent{
		ID:          row[0].AsString(),
		ChainID:     row[1].AsString(),
		StepID:      row[2].AsString(),
		Sequence:    row[3].AsUint64(),
		EventType:   row[4].AsString(),
		CreatedAtUS: row[5].AsUint64(),
		PayloadJSON: row[6].AsString(),
	}
}

func ChainEventID(chainID string, sequence uint64) string {
	return fmt.Sprintf("%s:%020d", stableID(chainID), sequence)
}

func ChainStepID(parts ...string) string {
	return stableID(strings.Join(parts, "\x00"))
}
