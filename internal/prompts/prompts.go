// Package prompts owns fixed short instructions and the authoritative rule
// materials that projects prepare through their normal versioned Git workflow.
package prompts

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

//go:embed *.md materials/*.md
var files embed.FS

func render(name string, data any) (string, error) {
	source, err := files.ReadFile(name + ".md")
	if err != nil {
		return "", fmt.Errorf("prompt %s: %w", name, err)
	}
	return renderText(name, string(source), data)
}
func renderText(name, source string, data any) (string, error) {
	source = strings.TrimSuffix(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	t, err := template.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", fmt.Errorf("prompt %s: %w", name, err)
	}
	var out bytes.Buffer
	if err = t.Execute(&out, data); err != nil {
		return "", fmt.Errorf("prompt %s: %w", name, err)
	}
	return out.String(), nil
}
func TestingRules() ([]byte, error)         { return files.ReadFile("materials/testing.md") }
func RoadmapParseRules() ([]byte, error)    { return files.ReadFile("materials/roadmap-parse.md") }
func RoadmapDocumentRules() ([]byte, error) { return files.ReadFile("materials/roadmap-documents.md") }
func RoadmapSpecGuide() ([]byte, error)     { return files.ReadFile("materials/roadmap-spec-guide.md") }
func MainAgentRules() ([]byte, error)       { return files.ReadFile("materials/main-agent.md") }
func MaintenanceRules() ([]byte, error)     { return files.ReadFile("materials/maintenance.md") }
func MaintenanceDecision() (string, error)  { return render("maintenance-decision", nil) }
func MaintenanceExecute() (string, error)   { return render("maintenance-execute", nil) }
func MainAgentAdvisory(context string) (string, error) {
	rules, err := MainAgentRules()
	if err != nil {
		return "", err
	}
	return render("main-agent", struct{ Rules, Context string }{string(rules), context})
}

// SameRules tolerates only checkout line endings on fixed rule copies.
// Dynamic JSON must instead match its original bytes; Roadmap Markdown has its
// own checkout-line-ending comparison without changing the original snapshots.
func SameRules(actual, expected []byte) bool {
	return len(actual) > 0 && bytes.Equal(bytes.ReplaceAll(actual, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n")))
}

func TestingReadOnly() (string, error) { return render("testing-readonly", nil) }
func TestingContext(snapshot []byte) (string, error) {
	var plan testmodel.Snapshot
	if err := json.Unmarshal(snapshot, &plan); err != nil {
		return "", err
	}
	if plan.Status != "ready" || plan.Commit == "" || plan.Project.Version != 1 {
		return "", errors.New("ready frozen test arrangement is required")
	}
	projectRaw, _ := json.Marshal(plan.Project)
	if _, err := testmodel.ParseProjectEntry(projectRaw); err != nil {
		return "", err
	}
	arrangement, err := testmodel.FrozenArrangement(plan)
	if err != nil {
		return "", err
	}
	fileSum := sha256.Sum256(arrangement)
	return render("testing-execute", struct{ FileSHA256 string }{hex.EncodeToString(fileSum[:])})
}
func TestingStep(body string, snapshot []byte, writesTests bool) (string, error) {
	if body == "" {
		return "", errors.New("original workflow prompt body is required")
	}
	var context string
	var err error
	if writesTests {
		context, err = TestingContext(snapshot)
	} else {
		context, err = TestingReadOnly()
	}
	if err != nil {
		return "", err
	}
	return render("testing-step", struct{ Body, Context string }{body, context})
}
func EnvironmentInvoke(interpreter, script string) (string, error) {
	if script == "" || (interpreter != "powershell.exe" && interpreter != "bash") {
		return "", errors.New("fixed environment invocation is required")
	}
	return render("environment-invoke", struct{ Interpreter, Script string }{interpreter, script})
}
func EnvironmentRepair(documentPath string) (string, error) {
	if documentPath == "" {
		return "", errors.New("environment preparation document path is required")
	}
	return render("environment-repair", struct{ DocumentPath string }{documentPath})
}
func RoadmapParse(source, commit string) (string, error) {
	if source == "" || commit == "" {
		return "", errors.New("fixed roadmap source and commit are required")
	}
	return render("roadmap-parse", struct{ Source, Commit string }{source, commit})
}
func RoadmapDocuments(commit, source, key string) (string, error) {
	if source == "" || commit == "" {
		return "", errors.New("fixed roadmap source and commit are required")
	}
	if strings.TrimSpace(key) == "" {
		return "", errors.New("target task key is required")
	}
	return render("roadmap-documents", struct{ Source, Commit, Key string }{source, commit, key})
}

// PRConflictResolution supplies the complete caller instruction and original PR facts.
func PRConflictResolution(pr, base string) (string, error) {
	if pr == "" || base == "" {
		return "", errors.New("original PR and base branch are required")
	}
	return render("pr_conflict_resolution", struct{ PR, Base string }{pr, base})
}
