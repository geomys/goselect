// Package godebug is a stand-in for the standard library's internal/godebug,
// which can't be imported outside the standard library.
//
// Settings are read from the GODEBUG environment variable only, once.
package godebug

import (
	"os"
	"strings"
	"sync"
)

// A Setting is a single setting in the $GODEBUG environment variable.
type Setting struct {
	name  string
	once  sync.Once
	value string
}

// New returns a new Setting for the $GODEBUG setting with the given name.
// A leading "#" marks an undocumented setting, as in internal/godebug.
func New(name string) *Setting {
	return &Setting{name: name}
}

// Name returns the name of the setting, without any leading "#".
func (s *Setting) Name() string {
	return strings.TrimPrefix(s.name, "#")
}

// Value returns the current value of the setting.
// When a setting appears more than once in $GODEBUG, the last one wins.
func (s *Setting) Value() string {
	s.once.Do(func() {
		for kv := range strings.SplitSeq(os.Getenv("GODEBUG"), ",") {
			if k, v, ok := strings.Cut(kv, "="); ok && k == s.Name() {
				s.value = v
			}
		}
	})
	return s.value
}

// IncNonDefault is a no-op: goup doesn't record runtime metrics.
func (s *Setting) IncNonDefault() {}
