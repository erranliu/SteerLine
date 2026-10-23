package testmanagement

import (
	"encoding/json"
	"errors"
	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"regexp"
	"strings"
)

var recordID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var summarySHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

func NewRecordID() string { return randomID() }

func ParseReport(raw []byte) (Report, error) {
	if len(raw) > m.MaxReportBytes {
		return Report{}, errors.New("test report exceeds 128 KiB")
	}
	r, err := parseReport(raw)
	if err != nil {
		return r, err
	}
	if !recordID.MatchString(r.ID) || len(r.Selection) != 1 || len(r.Results) != 1 || len(r.Definitions) != 1 || r.Definition.ID != r.Selection[0] || r.Definitions[r.Definition.ID] != Digest(r.Definition) {
		return r, errors.New("record requires one frozen test definition and a 32-hex report ID")
	}
	body, _ := json.Marshal(Catalogue{Tests: []Definition{r.Definition}})
	if _, err = ParseCatalogue(body); err != nil {
		return r, err
	}
	if r.StartedAt.IsZero() || r.FinishedAt.Before(r.StartedAt) || r.Command == "" || r.Repository == "" {
		return r, errors.New("record requires execution time, command and repository")
	}
	for _, sha := range []string{r.BeforeCommit, r.AfterCommit} {
		if sha != "" && !regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`).MatchString(sha) {
			return r, errors.New("invalid source commit")
		}
	}
	if r.Environment != r.Definition.Environment {
		return r, errors.New("actual environment differs from frozen definition")
	}
	if r.Evidence.Path != "summary.json" || r.Results[0].Log != "summary.json" || r.Evidence.Bytes < 0 || r.Evidence.SHA256 != "" && !summarySHA.MatchString(r.Evidence.SHA256) {
		return r, errors.New("record evidence must declare summary.json and its content digest")
	}
	if r.Scope != nil {
		seen := map[string]bool{}
		failed, incomplete := false, false
		for _, v := range r.Scope.Items {
			if strings.TrimSpace(v.ID) == "" || seen[v.ID] || v.Status != "passed" && v.Status != "failed" && v.Status != "incomplete" || v.Status != "passed" && strings.TrimSpace(v.Message) == "" {
				return r, errors.New("invalid scope result")
			}
			seen[v.ID] = true
			failed = failed || v.Status == "failed"
			incomplete = incomplete || v.Status == "incomplete"
		}
		if len(seen) == 0 || failed && r.Results[0].Status != "failed" || !failed && incomplete && r.Results[0].Status != "incomplete" || !failed && !incomplete && r.Results[0].Status != "passed" {
			return r, errors.New("scope and overall result differ")
		}
	}
	return r, nil
}
