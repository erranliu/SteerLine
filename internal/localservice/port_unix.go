//go:build !windows

package localservice

func portOwner(string) string { return "" }
