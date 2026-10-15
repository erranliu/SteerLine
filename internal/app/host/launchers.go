package host

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed launcher/*
var launcherFS embed.FS

// InstallLaunchers writes the application, Web and service-window entry points plus
// their README into the caller's application directory. Templates are stored
// LF and written CRLF so they behave as native .bat files.
func InstallLaunchers(dir string) ([]string, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("launcher directory is required")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	entries, e := fs.ReadDir(launcherFS, "launcher")
	if e != nil {
		return nil, e
	}
	var written []string
	for _, en := range entries {
		if en.IsDir() {
			continue
		}
		b, e := fs.ReadFile(launcherFS, "launcher/"+en.Name())
		if e != nil {
			return written, e
		}
		if filepath.Ext(en.Name()) == ".bat" {
			b = toCRLF(b)
		}
		dst := filepath.Join(dir, en.Name())
		if e := AtomicWrite(dst, b); e != nil {
			return written, e
		}
		written = append(written, dst)
	}
	return written, nil
}

func toCRLF(b []byte) []byte {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
}
