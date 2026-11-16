package manualverification

import (
	"strings"
	"testing"
)

func TestManualChecksDefinition(t *testing.T) {
	const valid = `{"version":1,"launch":{"program":"node","args":["preview.js","${PORT}"],"url":"http://127.0.0.1:${PORT}/preview"},"checks":[{"id":"M01","title":"Preview","steps":["Open it"],"expected":"It renders"}]}`
	d, err := Decode([]byte(valid))
	if err != nil || d.Launch.Cwd != "" || len(d.Checks) != 1 {
		t.Fatalf("valid definition: %#v %v", d, err)
	}
	for name, input := range map[string]string{
		"case folded":       strings.Replace(valid, `"version"`, `"Version"`, 1),
		"wrong version":     strings.Replace(valid, `"version":1`, `"version":1.0`, 1),
		"duplicate key":     strings.Replace(valid, `"version":1`, `"version":1,"version":1`, 1),
		"null arg":          strings.Replace(valid, `"preview.js"`, `null`, 1),
		"null cwd":          strings.Replace(valid, `"program":"node"`, `"program":"node","cwd":null`, 1),
		"escape cwd":        strings.Replace(valid, `"program":"node"`, `"program":"node","cwd":"../worktree"`, 1),
		"escape executable": strings.Replace(valid, `"node"`, `"./../node"`, 1),
		"remote URL":        strings.Replace(valid, `127.0.0.1`, `example.org`, 1),
		"bad placeholder":   strings.Replace(valid, `${PORT}`, `${OTHER}`, 1),
		"unexpected field":  strings.Replace(valid, `"program":"node"`, `"program":"node","command":"node"`, 1),
		"duplicate ID":      strings.Replace(valid, `"It renders"}]`, `"It renders"},{"id":"M01","title":"Two","steps":["Open"],"expected":"Two"}]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(input)); err == nil {
				t.Fatal("accepted invalid definition")
			}
		})
	}
}
