// Package counter is a no-op stand-in for cmd/internal/telemetry/counter.
// goup doesn't record telemetry; the toolchain it runs does.
package counter

// A Counter is a no-op telemetry counter.
type Counter struct{}

// New returns a no-op counter.
func New(name string) *Counter { return &Counter{} }

// Inc does nothing.
func (*Counter) Inc() {}

// Inc does nothing.
func Inc(name string) {}
