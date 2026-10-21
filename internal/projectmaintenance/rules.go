// Package projectmaintenance validates the actual inputs of automatic test generation.
package projectmaintenance

import (
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
func GenerationSources(plan string, r Reader) ([]tm.Source, error) {
	if _, e := tm.ArrangementPath(plan); e != nil {
		return nil, e
	}
	sources := []tm.Source{}
	for _, path := range []string{plan, tm.RulesPath, tm.CataloguePath} {
		b, e := Read(r, path)
		if e != nil {
			return nil, e
		}
		if path == tm.CataloguePath {
			if _, e = tm.ParseCatalogue(b); e != nil {
				return nil, e
			}
		}
		sources = append(sources, tm.Source{Path: path, Digest: pm.Digest(b), TextDigest: pm.TextDigest(b)})
	}
	return sources, nil
}
