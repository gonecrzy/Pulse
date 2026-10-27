package config

import (
	"path/filepath"
	"testing"
)

// Keep temporary persistence fixtures from importing an operator's global
// legacy key. Tests that need migration behavior should provide that source
// explicitly rather than inheriting process-wide machine state.
func isolateLegacyKeyLookupForTest(t *testing.T) {
	t.Helper()
	t.Setenv("PULSE_LEGACY_KEY_PATH", filepath.Join(t.TempDir(), "absent-legacy-key"))
}
