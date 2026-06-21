package engine

import (
	"context"
)

type QueryPool struct {
	sem chan struct{}
}

func NewQueryPool(maxConcurrent int) *QueryPool {
	return &QueryPool{
		sem: make(chan struct{}, maxConcurrent),
	}
}

func (p *QueryPool) Execute(ctx context.Context, fn func() error) error {
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}