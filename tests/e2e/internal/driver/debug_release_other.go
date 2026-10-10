//go:build !windows

package driver

func debugPlainFile(string) error { return nil }
func debugVisibleConsole() bool   { return false }
