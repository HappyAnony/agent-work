package model

import "context"

// One gate per owner, shared by attribute mutations and their derived refresh.
// It is not a substitute for R04's source-wide commit/CAS boundary.
type attributeMutationGate struct{ token chan struct{} }

func newAttributeMutationGate() *attributeMutationGate {
	g := &attributeMutationGate{token: make(chan struct{}, 1)}
	g.token <- struct{}{}
	return g
}
func (g *attributeMutationGate) with(ctx context.Context, fn func() error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.token:
	}
	defer func() { g.token <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

// Only the explicit single-workspace compatibility adapter uses this gate.
var legacyAttributeMutationGate = newAttributeMutationGate()
