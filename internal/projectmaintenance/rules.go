// Package projectmaintenance reads the committed task test arrangement.
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
	b, err := r(path)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b) > tm.MaxDocumentBytes || !utf8.Valid(b) {
		return nil, fmt.Errorf("invalid bounded UTF-8 material: %s", path)
	}
	return b, nil
}

// Arrangement expands only committed IDs. The caller pins every read to commit.
// Basis deliberately excludes unselected catalogue entries.
func Arrangement(plan, commit string, r Reader, protocol []byte) (tm.Snapshot, []string, error) {
	out := tm.Snapshot{PlanPath: plan, Commit: commit, Status: "invalid"}
	name, err := tm.ArrangementPath(plan)
	if err != nil {
		return out, nil, err
	}
	entry, err := tm.ReadProjectEntry(r)
	if err != nil {
		return out, nil, err
	}
	out.Project = entry
	basis := []string{}
	seen := map[string]bool{}
	var planText, rawArrangement, rawCatalogue []byte
	for _, path := range append([]string{plan, name, tm.ProtocolPath}, entry.Paths()...) {
		if seen[path] {
			continue
		}
		seen[path] = true
		body, err := Read(r, path)
		if err != nil {
			return out, nil, fmt.Errorf("%s: %w", path, err)
		}
		switch path {
		case plan:
			planText = body
		case name:
			rawArrangement = body
		case entry.TestingCatalogue:
			rawCatalogue = body
		case tm.ProtocolPath:
			if len(protocol) == 0 || !bytes.Equal(bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(protocol, []byte("\r\n"), []byte("\n"))) {
				return out, nil, fmt.Errorf("fixed testing protocol differs: %s", path)
			}
		}
		if path != entry.TestingCatalogue && path != name {
			basis = append(basis, path, pm.TextDigest(body))
		}
	}
	catalogue, err := tm.ParseCatalogue(rawCatalogue)
	if err != nil {
		return out, nil, err
	}
	out.Definitions, err = tm.Select(rawArrangement, catalogue)
	if err != nil {
		return out, nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(out.Definitions) == 0 && !tm.AllowsNoTests(string(planText)) {
		return out, nil, fmt.Errorf("%s: 空测试安排需要 Plan 明确说明无需自动测试", name)
	}
	out.Digest = pm.Hash(out.Definitions)
	out.Status = "ready"
	return out, append(basis, name, out.Digest), nil
}
