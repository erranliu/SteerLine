package console

import (
	"os"
	"path/filepath"
)

func writeViewAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".view-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceViewFile(name, path)
}
