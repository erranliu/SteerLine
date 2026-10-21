package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/app"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type managedSource struct {
	commit string
	clean  bool
}

// A Git diagnostic is never a commit/branch value. Unknown metadata remains
// empty so a real execution can still be retained as an unfixed-source record.
func managedGitValue(args ...string) string {
	body, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func observeManagedSource() managedSource {
	commit := managedGitValue("rev-parse", "HEAD")
	status, err := exec.Command("git", "status", "--porcelain", "--untracked-files=normal").Output()
	return managedSource{commit: commit, clean: err == nil && (len(commit) == 40 || len(commit) == 64) && strings.TrimSpace(string(status)) == ""}
}

type managedRun struct {
	started                     time.Time
	definitions                 map[string]testmanagement.Definition
	environments                map[string]string
	ids                         map[string]string
	reruns                      map[string]string
	repository, branch, command string
}

func freezeManagedReports(opts options, sequence uint64) (*managedRun, error) {
	root, err := e2eRoot()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testmanagement.CataloguePath)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "registered tests: catalogue unavailable:", err)
		return nil, err
	}
	catalog, err := testmanagement.ParseCatalogue(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "registered tests:", err)
		return nil, err
	}
	index := map[string]testmanagement.Definition{}
	for _, d := range catalog.Tests {
		index[d.ID] = d
	}
	var selected []string
	actualEnvironments := map[string]string{}
	if opts.level == "L1" {
		selected = []string{"l1.minimum"}
		actualEnvironments["l1.minimum"] = "isolated"
	} else {
		scenarios, e := selectE2E(root, opts)
		if e != nil {
			fmt.Fprintln(os.Stderr, "registered selection:", e)
			return nil, e
		}
		for _, entry := range scenarios {
			selected = append(selected, entry.ID)
			actualEnvironments[entry.ID] = entry.Env
		}
	}

	run := &managedRun{started: time.Now().UTC(), definitions: map[string]testmanagement.Definition{}, environments: actualEnvironments, ids: map[string]string{}, reruns: map[string]string{}, repository: managedGitValue("rev-parse", "--path-format=absolute", "--git-common-dir"), branch: managedGitValue("branch", "--show-current"), command: strings.Join(os.Args, " ")}
	if run.repository == "" {
		run.repository = root
	}
	for _, id := range selected {
		d, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("unregistered test %s", id)
		}
		if d.Environment != actualEnvironments[id] {
			return nil, fmt.Errorf("registered environment differs from selected test %s", id)
		}
		run.definitions[id] = d
		run.ids[id] = testmanagement.NewRecordID()
	}
	if opts.rerunOf != "" {
		for _, mapping := range strings.Split(opts.rerunOf, ",") {
			parts := strings.Split(mapping, "=")
			if len(parts) != 2 || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(parts[1]) || run.reruns[parts[0]] != "" {
				return nil, fmt.Errorf("invalid rerun-of mapping")
			}
			if _, ok := run.ids[parts[0]]; !ok {
				return nil, fmt.Errorf("rerun-of references unselected test")
			}
			run.reruns[parts[0]] = parts[1]
		}
	}
	return run, nil
}

// Existing execution and exit codes remain authoritative. This only emits a
// small registered report beside the original summary/logs, even after failure.
func writeManagedReports(opts options, before managedSource, sequence uint64) {
	if opts.level == "" || opts.buildOnly || opts.summarizeLog != "" || len(opts.goTestArgs) > 0 {
		return
	}
	run := opts.managed
	if run == nil {
		return
	}
	index, actualEnvironments := run.definitions, run.environments
	var selected []string
	for id := range index {
		selected = append(selected, id)
	}
	sort.Strings(selected)
	var err error
	summaryRaw, readErr := os.ReadFile(filepath.Join(opts.out, "summary.json"))
	var scoped scopedReport
	var e2e e2eReport
	if opts.level == "L1" {
		err = json.Unmarshal(summaryRaw, &scoped)
	} else {
		err = json.Unmarshal(summaryRaw, &e2e)
	}
	after := observeManagedSource()
	for _, id := range selected {
		d, ok := index[id]
		if !ok {
			fmt.Fprintln(os.Stderr, "registered test missing from catalogue:", id)
			continue
		}
		status, message := "incomplete", "测试报告缺失或不完整"
		if readErr == nil && err == nil {
			if opts.level == "L1" {
				count, valid, assertionFailed := 0, true, false
				for _, group := range scoped.Groups {
					count += len(group.Summary.Tests)
					for _, test := range group.Summary.Tests {
						assertionFailed = assertionFailed || test.Status == "fail"
					}
					if !group.Summary.Complete || group.Summary.SkippedTests > 0 || len(group.Summary.SkippedPackages) > 0 {
						valid = false
					}
				}
				if scoped.Scope == "quick" && assertionFailed {
					status, message = "failed", "L1 存在实际断言失败；完整性与原始错误见 summary.json"
				} else if scoped.Complete && scoped.Scope == "quick" && count > 0 && valid {
					status = "failed"
					message = "L1 存在失败"
					if scoped.Passed {
						status = "passed"
						message = "固定 L1 选择通过"
					}
				}
			} else {
				var evidence struct {
					ID      string `json:"id"`
					Passed  bool   `json:"passed"`
					Cleanup bool   `json:"cleanup_complete"`
				}
				body, e := os.ReadFile(filepath.Join(opts.out, "scenarios", id, "scenario.json"))
				matched := ""
				for _, entry := range e2e.Selected {
					if entry.ID == id && entry.Env == d.Environment && entry.Level == d.Level {
						for _, group := range e2e.Groups {
							if !group.Summary.Complete {
								continue
							}
							for _, test := range group.Summary.Tests {
								if test.Package == strings.TrimPrefix(entry.Package, "./") || strings.HasSuffix(test.Package, strings.TrimPrefix(entry.Package, ".")) {
									if test.Name == entry.Test && (test.Status == "pass" || test.Status == "fail") {
										matched = test.Status
									}
								}
							}
						}
					}
				}
				if e == nil && json.Unmarshal(body, &evidence) == nil && evidence.ID == id && evidence.Cleanup && matched != "" {
					status = "failed"
					message = "场景断言失败"
					if matched == "pass" && evidence.Passed && e2e.Cleanup {
						status = "passed"
						message = "所选场景与清理通过"
					}
				}
				for _, blocked := range e2e.Blocked {
					if blocked.ID == id {
						status = "incomplete"
						message = "场景前置未满足"
					}
				}
			}
		}
		report := testmanagement.Report{ID: run.ids[id], StartedAt: run.started, FinishedAt: time.Now().UTC(), Definition: d, Command: run.command, Repository: run.repository, Branch: run.branch, RerunOf: run.reruns[id], Evidence: tm.ReportEvidence{Path: "summary.json", Bytes: len(summaryRaw), SHA256: testmanagement.ContentDigest(summaryRaw)}, Sequence: sequence, Selection: []string{id}, Environment: actualEnvironments[id], Definitions: map[string]string{id: testmanagement.Digest(d)}, BeforeCommit: before.commit, AfterCommit: after.commit, BeforeClean: before.clean, AfterClean: after.clean, Results: []testmanagement.Result{{TestID: id, Status: status, Message: message, Log: "summary.json"}}}
		if e := testmanagement.WriteRecordJSON(filepath.Join(opts.out, "managed-"+id+".json"), report); e != nil {
			fmt.Fprintln(os.Stderr, "write registered report:", e)
		}
	}
	if opts.recordProject != "" {
		registerManagedReports(opts)
	} else {
		fmt.Fprintln(os.Stderr, "测试报告已保存；未指定 record-project，尚未登记。")
	}
}
func registerManagedReports(opts options) {
	items, e := testmanagement.ReadRegistrationDirectory(opts.out)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, connectErr := app.OpenApplicationProject(ctx, opts.recordProject, "")
	if p != nil {
		defer p.Close()
	}
	receipts, err := testmanagement.RegisterFiles(opts.out, items, func(in tm.RegisterRecord, requestID string) testmanagement.RegistrationResult {
		callErr := connectErr
		var result json.RawMessage
		if callErr == nil {
			result, callErr = p.RegisterTestRecord(ctx, api.TestRecordRegistration(in), requestID)
		}
		outcome := testmanagement.RegistrationResult{Status: applicationclient.RecordRegistrationStatus(callErr), Result: result}
		if callErr != nil {
			outcome.Diagnostic = callErr.Error()
		}
		return outcome
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "测试结果保留，登记未完成：", err)
		return
	}
	for _, receipt := range receipts {
		if receipt.Status != "registered" {
			fmt.Fprintln(os.Stderr, "测试结果保留，登记未完成：", receipt.Path, receipt.Diagnostic)
		}
	}
}

func managedReportSequence() uint64 { return uint64(time.Now().UnixNano()) }
