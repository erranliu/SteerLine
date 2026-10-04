//go:build !windows

package controlled

import "os"

func replaceStateFile(from, to string) error { return os.Rename(from, to) }
