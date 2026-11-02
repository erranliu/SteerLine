// Package projectmaintenance validates the actual inputs of automatic test generation.
package projectmaintenance

import (
	"bytes"
	"fmt"
	pm "github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"unicode/utf8"
)

type Reader func(string) ([]byte, error)

func Read(r Reader, path string) ([]byte, error) {
	b, e := r(path)
	if e != nil {
		return nil, e
	}
	if len(b) == 0 || len(b) > tm.MaxDocumentBytes || !utf8.Valid(b) {
		return nil, fmt.Errorf("invalid bounded UTF-8 material: %s", path)
	}
	return b, nil
}

// GenerationSources never reads an arrangement or approves content. Already
// merged Plan/rules/catalogue are the only generation inputs.
func GenerationSources(plan string, r Reader, protocol []byte) ([]tm.Source, error) {
	if len(protocol) == 0 {
		return nil, fmt.Errorf("fixed testing protocol is required")
	}
	if _, e := tm.ArrangementPath(plan); e != nil {
		return nil, e
	}
	sources := []tm.Source{}
	entry, e := tm.ReadProjectEntry(r)
	if e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	for _, path := range append([]string{plan, tm.ProtocolPath}, entry.Paths()...) {
		if seen[path] {
			continue
		}
		seen[path] = true
		b, e := Read(r, path)
		if e != nil {
			return nil, e
		}
		if path == tm.ProtocolPath {
			if !bytes.Equal(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(protocol, []byte("\r\n"), []byte("\n"))) {
				return nil, fmt.Errorf("fixed testing protocol differs: %s", path)
			}
		}
		if path == entry.TestingCatalogue {
			if _, e = tm.ParseCatalogue(b); e != nil {
				return nil, e
			}
		}
		sources = append(sources, tm.Source{Path: path, Digest: pm.Digest(b), TextDigest: pm.TextDigest(b)})
	}
	return sources, nil
}
