package refactoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/refactoring/model"
)

func TestDirectionCatalogueAndVersion(t *testing.T) {
	home := t.TempDir()
	builtIn := ListDirections(home)
	if len(builtIn.Directions) != 5 || len(builtIn.Diagnostics) != 0 {
		t.Fatal(builtIn)
	}
	dir := filepath.Join(home, customDirectionsDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const original = "\ufeff# 收拢测试夹具\r\n\r\n优先处理共享夹具。  \r\n"
	write("simplify.md", original)
	write("invalid.md", "# 缺少正文\n")
	write("ignored.txt", "# 忽略\n内容")
	catalogue := ListDirections(home)
	if len(catalogue.Directions) != 6 || len(catalogue.Diagnostics) != 1 {
		t.Fatal(catalogue)
	}
	want := model.Direction{ID: "custom:simplify", Name: "收拢测试夹具", Prompt: original, Revision: model.Digest([]byte(original))}
	if catalogue.Directions[5] != want {
		t.Fatalf("catalogue lost original markdown: %#v", catalogue.Directions[5])
	}
	got, err := ResolveDirection(home, want.ID, want.Revision)
	if err != nil || got != want {
		t.Fatal(got, err)
	}
	write("simplify.md", "# 已改名\n新的正文。\n")
	if _, err := ResolveDirection(home, want.ID, want.Revision); err == nil || !strings.Contains(err.Error(), "已更新") {
		t.Fatal("stale revision accepted", err)
	}
	if err := os.Remove(filepath.Join(dir, "simplify.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveDirection(home, want.ID, want.Revision); err == nil {
		t.Fatal("missing direction accepted")
	}
	write("SIMPLIFY.md", original)
	if _, err := ResolveDirection(home, want.ID, want.Revision); err == nil {
		t.Fatal("case-renamed direction accepted under the old catalogue ID")
	}
	fixed := builtIn.Directions[0]
	if got, err := ResolveDirection(home, fixed.ID, fixed.Revision); err != nil || got != fixed {
		t.Fatal(got, err)
	}
}
