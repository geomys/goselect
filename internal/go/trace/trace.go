// Package trace is a no-op stand-in for cmd/go/internal/trace.
package trace

import "context"

// A Span is a no-op trace span.
type Span struct{}

// StartSpan returns ctx and a no-op span.
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	return ctx, &Span{}
}

// Done does nothing.
func (*Span) Done() {}
