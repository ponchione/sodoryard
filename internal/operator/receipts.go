package operator

import (
	"context"
	"errors"
	"fmt"

	"github.com/ponchione/sodoryard/internal/chain"
)

func (s *Service) ReadReceipt(ctx context.Context, chainID string, step string) (ReceiptView, error) {
	path := fmt.Sprintf("receipts/orchestrator/%s.md", chainID)
	if step != "" {
		store, err := s.store()
		if err != nil {
			return ReceiptView{}, err
		}
		steps, err := store.ListSteps(ctx, chainID)
		if err != nil {
			return ReceiptView{}, err
		}
		if stepPath, ok := receiptPathForStep(chainID, step, steps); ok {
			path = stepPath
		}
	}
	backend, err := s.brainBackend()
	if err != nil {
		return ReceiptView{}, err
	}
	content, err := backend.ReadDocument(ctx, path)
	if err != nil {
		return ReceiptView{}, err
	}
	return ReceiptView{ChainID: chainID, Step: step, Path: path, Content: content}, nil
}

func receiptPathForStep(chainID string, step string, steps []chain.Step) (string, bool) {
	for _, candidate := range steps {
		if fmt.Sprintf("%d", candidate.SequenceNum) == step {
			return candidate.ReceiptPath, true
		}
	}
	return fmt.Sprintf("receipts/orchestrator/%s.md", chainID), false
}

func (s *Service) brainBackend() (interface {
	ReadDocument(context.Context, string) (string, error)
}, error) {
	if s == nil || s.rt == nil {
		return nil, errors.New("operator service is closed")
	}
	if s.rt.BrainBackend == nil {
		return nil, errors.New("operator runtime brain backend is nil")
	}
	return s.rt.BrainBackend, nil
}
