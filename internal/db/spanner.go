package db

import (
	"context"

	"cloud.google.com/go/spanner"
)

// NewSpannerClient creates a new Spanner client for the given database string.
func NewSpannerClient(ctx context.Context, db string) (*spanner.Client, error) {
	return spanner.NewClient(ctx, db)
}
