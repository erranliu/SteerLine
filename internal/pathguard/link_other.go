//go:build !windows && !unix

package pathguard

import (
	"errors"
	"os"
)

func fileLinkCount(path string) (uint64, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return 0, errors.New("SQLite file link inspection is unsupported on this platform")
}
