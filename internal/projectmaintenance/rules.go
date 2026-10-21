// Package projectmaintenance implements the same bounded rules for initialization,
// baseline and local scope. Callers supply bytes; rules never initialize state.
package projectmaintenance

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/projectmaintenance/model"
	testmanagement "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

type Reader func(string) ([]byte, error)
type Item struct {
	// Definitions are the selected values from this exact assessment, reused at first reservation.
	Definitions      []testmanagement.Definition `json:"-"`
	Plan             string                      `json:"plan"`
	Target           string                      `json:"target"`
	Status           string                      `json:"status"`
	Diagnostic       string                      `json:"diagnostic,omitempty"`
	Inputs           string                      `json:"inputs,omitempty"`
	Output           string                      `json:"output,omitempty"`
	ReviewDigest     string                      `json:"review_digest,omitempty"`
	ApprovalRevision uint64                      `json:"approval_revision"`
	Tests            []string                    `json:"tests"`
}
type Assessment struct {
	Mode   string `json:"mode"`
	Commit string `json:"commit"`
	Items  []Item `json:"items"`
}

func Read(r Reader, path string) ([]byte, error) {
	b, e := r(path)
	if e != nil {
		return nil, e
	}
	if len(b) == 0 || len(b) > testmanagement.MaxDocumentBytes || !utf8.Valid(b) {
		return nil, fmt.Errorf("invalid bounded UTF-8 material: %s", path)
	}
	return b, nil
}
func Evaluate(mode, commit string, plans []string, r Reader, approvals map[string]model.Approval) (Assessment, error) {
	if mode != "initialize" && mode != "baseline" && mode != "local" {
		return Assessment{}, errors.New("unknown maintenance mode")
	}
	a := Assessment{Mode: mode, Commit: commit, Items: []Item{}}
	// A project without Plans still checks the common materials; never invent a Task.
	rules, re := Read(r, testmanagement.RulesPath)
	catalogue, ce := Read(r, testmanagement.CataloguePath)
	catalog, pe := testmanagement.ParseCatalogue(catalogue)
	if len(plans) == 0 {
		i := Item{Status: "ready", Tests: []string{}}
		for _, e := range []error{re, ce, pe} {
			if e != nil {
				i.Status = "invalid"
				i.Diagnostic = e.Error()
				break
			}
		}
		a.Items = append(a.Items, i)
		return a, nil
	}
	seen := map[string]bool{}
	for _, plan := range plans {
		if seen[plan] {
			continue
		}
		seen[plan] = true
		i := Item{Plan: plan, Status: "invalid", Tests: []string{}}
		target, e := testmanagement.ArrangementPath(plan)
		if e != nil {
			return a, e
		}
		i.Target = target
		body, be := Read(r, plan)
		output, oe := Read(r, target)
		var failure error
		for _, e := range []error{re, ce, pe, be, oe} {
			if e != nil {
				failure = e
				break
			}
		}
		if failure != nil {
			i.Diagnostic = failure.Error()
			a.Items = append(a.Items, i)
			continue
		}
		definitions, e := testmanagement.Select(output, catalog)
		if e != nil {
			i.Diagnostic = e.Error()
			a.Items = append(a.Items, i)
			continue
		}
		if len(definitions) == 0 && !strings.Contains(string(body), "无需自动测试") && !strings.Contains(strings.ToLower(string(body)), "no automated tests") {
			i.Diagnostic = "空安排需要 Plan 明确说明无需自动测试"
			a.Items = append(a.Items, i)
			continue
		}
		i.Definitions = definitions
		for _, d := range definitions {
			i.Tests = append(i.Tests, d.ID)
		}
		i.Inputs = model.Hash([]string{model.RulesVersion, model.TextDigest(body), model.TextDigest(rules), model.TextDigest(catalogue)})
		i.Output = model.TextDigest(output)
		old := approvals[plan]
		i.ApprovalRevision = old.Revision
		i.Status = "needs_review"
		if old.Inputs == i.Inputs && old.Output == i.Output {
			i.Status = "ready"
		}
		i.ReviewDigest = model.Hash([]string{commit, plan, i.Inputs, i.Output, fmt.Sprint(old.Revision)})
		a.Items = append(a.Items, i)
	}
	return a, nil
}
