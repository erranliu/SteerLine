//go:build linux

package host

func jobDiagnosticChildOutput() ([]byte, error) { return nil, nil }
func currentConsoleWindowForTests() uintptr     { return 0 }
func windowExistsForTests(uint64) bool          { return false }
func windowVisibleForTests(uint64) bool         { return false }
