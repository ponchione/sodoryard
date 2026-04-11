package codestore

import (
	"context"

	"github.com/ponchione/sodoryard/internal/codeintel"
	"github.com/ponchione/sodoryard/internal/vectorstore"
)

func Open(ctx context.Context, path string) (codeintel.Store, error) {
	return vectorstore.NewStore(ctx, path)
}
