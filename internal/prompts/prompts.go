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
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"strings"
	"text/template"
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
func EnvironmentGenerationRules() ([]byte, error) {
	return files.ReadFile("materials/environment-generation.md")
}
func EnvironmentExecutionRules() ([]byte, error) {
	return files.ReadFile("materials/environment-execution.md")
}

// SameRules tolerates only checkout line endings on fixed rule copies.
// Dynamic data and original Markdown must instead match their original bytes.
func SameRules(actual, expected []byte) bool {
	return len(actual) > 0 && bytes.Equal(bytes.ReplaceAll(actual, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n")))
}

func TestingGenerate(plan, commit string) (string, error) {
	if plan == "" || commit == "" {
		return "", errors.New("fixed Plan and commit are required")
	}
	return render("testing-generate", struct{ Plan, Commit string }{plan, commit})
}
func TestingReadOnly() (string, error) { return render("testing-readonly", nil) }
func TestingContext(snapshot []byte, preparedWriter bool) (string, error) {
	if !preparedWriter {
		return "", errors.New("materials_not_ready: a verified project writer is required; inline fallback is disabled")
	}
	var plan testmodel.Snapshot
	if err := json.Unmarshal(snapshot, &plan); err != nil {
		return "", err
	}
	if plan.Status != "ready" || plan.Commit == "" {
		return "", errors.New("ready frozen test arrangement is required")
	}
	arrangement, err := testmodel.ArrangementPath(plan.PlanPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(snapshot)
	return render("testing-execute", struct{ Plan, Arrangement, Commit, Digest, Snapshot string }{plan.PlanPath, arrangement, plan.Commit, plan.Digest, hex.EncodeToString(sum[:])})
}
func TestingStep(body string, snapshot []byte, preparedWriter, writesTests bool) (string, error) {
	if body == "" {
		return "", errors.New("original workflow prompt body is required")
	}
	var context string
	var err error
	if writesTests {
		context, err = TestingContext(snapshot, preparedWriter)
	} else {
		context, err = TestingReadOnly()
	}
	if err != nil {
		return "", err
	}
	return render("testing-step", struct{ Body, Context string }{body, context})
}
func EnvironmentGenerate(commit string) (string, error) {
	if commit == "" {
		return "", errors.New("fixed environment commit is required")
	}
	return render("environment-generate", struct{ Commit string }{commit})
}
func EnvironmentPrepare() (string, error) { return render("environment-prepare", nil) }
func EnvironmentPublish() (string, error) { return render("environment-publish", nil) }
func RoadmapParse(source, commit string) (string, error) {
	if source == "" || commit == "" {
		return "", errors.New("fixed roadmap source and commit are required")
	}
	return render("roadmap-parse", struct{ Source, Commit string }{source, commit})
}
func RoadmapDocuments(commit, source string) (string, error) {
	if source == "" || commit == "" {
		return "", errors.New("fixed roadmap source and commit are required")
	}
	return render("roadmap-documents", struct{ Source, Commit string }{source, commit})
}
