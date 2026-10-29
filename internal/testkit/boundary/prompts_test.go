package boundary

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPromptGovernanceSourceBoundary(t *testing.T) {
	cases := []struct {
		name, file, code string
		rejected         bool
	}{
		{"registered-render", "internal/testmanagement/generation.go", `package p;func Generate(){instruction,_:=prompts.TestingGenerate();_ = G{Instruction:instruction}}`, false},
		{"direct-inline", "internal/testmanagement/generation.go", `package p;func Generate(){_ = G{Instruction:"new rule"}}`, true},
		{"local-inline", "internal/testmanagement/generation.go", `package p;func Generate(){instruction:="new rule";_ = G{Instruction:instruction}}`, true},
		{"direct-append", "internal/testmanagement/generation.go", `package p;func Generate(){_ = G{Instruction:prompts.TestingGenerate()+"extra"}}`, true},
		{"transformed", "internal/testmanagement/generation.go", `package p;func Generate(){body:=strings.ReplaceAll(original,"a","b");_ = G{Instruction:body}}`, true},
		{"append-assignment", "internal/testmanagement/generation.go", `package p;func Generate(){input.Instruction += "extra"}`, true},
		{"new-helper-inactive", "internal/helper/edge_plan9.go", "//go:build never_enabled\npackage p;func NewHelper(){_ = G{Instruction:body}}", true},
		{"new-override", "internal/new/edge.go", `package p;func NewHelper(){input.Workflow.PromptOverrides["publish"] = body}`, true},
		{"package-literal", "internal/new/edge.go", `package p;var input=G{Instruction:body}`, true},
		{"foreign-embed", "internal/new/edge.go", "package p\n//go:embed instructions.md\nvar text string", true},
		{"owned-embed", "internal/prompts/prompts.go", "package prompts\n//go:embed *.md\nvar text string", false},
		{"test-fixture", "internal/new/edge_test.go", `package p;func TestPrompt(){_ = G{Instruction:"fixture"}}`, false},
		{"preserve-schema", "internal/new/schema.go", `package p;var schema=G{OutputSchema:"schema"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			writeCallerSource(t, root, c.file, c.code)
			report, err := ScanSource(root)
			if err != nil {
				t.Fatal(err)
			}
			found := strings.Contains(strings.Join(report.Violations, "\n"), "prompt boundary:")
			if found != c.rejected {
				t.Fatalf("rejected=%v, violations=%v", c.rejected, report.Violations)
			}
		})
	}
	_, file, _, _ := runtime.Caller(0)
	report, err := ScanSource(filepath.Clean(filepath.Join(filepath.Dir(file), "../../..")))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range report.Violations {
		if strings.HasPrefix(v, "prompt boundary:") {
			t.Error(v)
		}
	}
}
