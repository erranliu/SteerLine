// Package prompts owns SteerLine's fixed model instructions. Business owners
// select stages and validate facts; this package only renders embedded text.
package prompts

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	testmodel "github.com/erranliu/SteerLine/internal/testmanagement/model"
	progresstools "github.com/erranliu/SteerLine/tools"
	"strings"
	"text/template"
)

//go:embed *.md
var files embed.FS

func render(name string, data any) (string, error) {
	source, err := files.ReadFile(name + ".md")
	if err != nil {
		return "", fmt.Errorf("prompt %s: %w", name, err)
	}
	return renderText(name, string(source), data)
}
func renderText(name, source string, data any) (string, error) {
	// Normalize fixed assets only, never inserted user text, facts or writer bytes.
	source = strings.TrimSuffix(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	t, err := template.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", fmt.Errorf("prompt %s: %w", name, err)
	}
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return "", fmt.Errorf("prompt %s: %w", name, err)
	}
	return out.String(), nil
}
func TestingGenerate() (string, error) { return render("testing-generate", nil) }
func TestingReadOnly() (string, error) { return render("testing-readonly", nil) }
func TestingContext(snapshot []byte, referencedWriter bool) (string, error) {
	facts, err := testmodel.TestingFacts(snapshot)
	if err != nil {
		return "", err
	}
	var writerContext string
	if referencedWriter {
		writerContext, err = render("testing-writer-reference", nil)
	} else {
		writerContext, err = render("testing-writer-inline", struct{ Writer string }{progresstools.PowerShellWriter})
	}
	if err != nil {
		return "", err
	}
	return render("testing-execute", struct{ Facts, WriterContext string }{string(facts), writerContext})
}

// TestingStep appends one stage-appropriate context to the original published/user
// body. It never searches or rewrites external text.
func TestingStep(body string, snapshot []byte, referencedWriter, writesTests bool) (string, error) {
	if body == "" {
		return "", errors.New("original workflow prompt body is required")
	}
	var context string
	var err error
	if writesTests {
		context, err = TestingContext(snapshot, referencedWriter)
	} else {
		context, err = TestingReadOnly()
	}
	if err != nil {
		return "", err
	}
	return render("testing-step", struct{ Body, Context string }{body, context})
}
func EnvironmentGenerate(commit, branch string, publication, previous json.RawMessage) (string, error) {
	if commit == "" || branch == "" || !json.Valid(publication) || len(previous) > 0 && !json.Valid(previous) {
		return "", errors.New("fixed environment commit, branch and JSON facts are required")
	}
	return render("environment-generate", struct{ Commit, Branch, Publication, Previous string }{commit, branch, string(publication), string(previous)})
}
func EnvironmentPrepare(diagnostic string) (string, error) {
	result, err := render("environment-result", nil)
	if err != nil {
		return "", err
	}
	return render("environment-prepare", struct{ Result, Diagnostic string }{result, diagnostic})
}
func EnvironmentPublish() (string, error) {
	result, err := render("environment-result", nil)
	if err != nil {
		return "", err
	}
	return render("environment-publish", struct{ Result string }{result})
}
func RoadmapParse() (string, error) { return render("roadmap-parse", nil) }
func RoadmapDocuments(commit string, guides json.RawMessage) (string, error) {
	if commit == "" || !json.Valid(guides) {
		return "", errors.New("fixed roadmap commit and guide facts are required")
	}
	return render("roadmap-documents", struct{ Commit, Guides string }{commit, string(guides)})
}
