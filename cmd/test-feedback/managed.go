package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/runtimeprogress"
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
	status, err := exec.Command("git", "status", "--porcelain", "--untracked-files=normal", "--", ".", ":(exclude)"+testmanagement.RecordsPath).Output()
	return managedSource{commit: commit, clean: err == nil && (len(commit) == 40 || len(commit) == 64) && strings.TrimSpace(string(status)) == ""}
}

type managedRun struct {
	started                     time.Time
	definitions                 map[string]testmanagement.Definition
	environments                map[string]string
	ids                         map[string]string
	repository, branch, command string
}

func freezeManagedReports(opts options) (*managedRun, error) {
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
	} else if def, ok := selectLocalSuite(opts); ok {
		selected = []string{def.ID}
		actualEnvironments[def.ID] = "isolated"
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

	run := &managedRun{started: time.Now().UTC(), definitions: map[string]testmanagement.Definition{}, environments: actualEnvironments, ids: map[string]string{}, repository: managedGitValue("rev-parse", "--path-format=absolute", "--git-common-dir"), branch: managedGitValue("branch", "--show-current"), command: strings.Join(os.Args, " ")}
	if run.repository == "" {
		run.repository = root
	}
	for _, id := range selected {
		d, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("unregistered test %s", id)
		}
		if d.Level != opts.level || d.Environment != actualEnvironments[id] {
			return nil, fmt.Errorf("registered level/environment differs from selected test %s", id)
		}
		run.definitions[id] = d
		run.ids[id] = testmanagement.NewRecordID()
	}
	values := map[string]bool{}
	for id := range run.ids {
		values[testmanagement.ProgressKey(id)] = false
	}
	if err := runtimeprogress.Update(context.Background(), values, true); err != nil {
		return nil, fmt.Errorf("reset runtime progress: %w", err)
	}
	return run, nil
}

// Preserve the execution result and report failures to the caller as well: a
// successful process cannot hide incomplete coverage or a missing managed file.
func writeManagedReports(opts options, before managedSource, sequence uint64) error {
	if opts.level == "" || opts.buildOnly || opts.summarizeLog != "" || len(opts.goTestArgs) > 0 {
		return nil
	}
	run := opts.managed
	if run == nil {
		return nil
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
	localDef, local := selectLocalSuite(opts)
	scopedRun := opts.level == "L1" || local
	if scopedRun {
		err = json.Unmarshal(summaryRaw, &scoped)
	} else {
		err = json.Unmarshal(summaryRaw, &e2e)
	}
	after := observeManagedSource()
	var failures []error
	for _, id := range selected {
		d, ok := index[id]
		if !ok {
			fmt.Fprintln(os.Stderr, "registered test missing from catalogue:", id)
			failures = append(failures, fmt.Errorf("unregistered test %s", id))
			continue
		}
		status, message := "incomplete", "测试报告缺失或不完整"
		if readErr == nil && err == nil {
			if scopedRun {
				expectedScope := "quick"
				if opts.level == "L4" {
					expectedScope = "local"
					if localDef.ID == "go.full" {
						expectedScope = "full"
					}
				}
				count, valid, assertionFailed := 0, true, false
				for _, group := range scoped.Groups {
					count += len(group.Summary.Tests)
					for _, test := range group.Summary.Tests {
						assertionFailed = assertionFailed || test.Status == "fail"
					}
					if !group.Summary.Complete || skippedCoverage(scoped, group) {
						valid = false
					}
				}
				if scoped.Scope == expectedScope && assertionFailed {
					status, message = "failed", "测试集存在实际断言失败；完整性与原始错误见 summary.json"
				} else if scoped.Complete && scoped.Scope == expectedScope && count > 0 && valid {
					status = "failed"
					message = "测试集存在失败"
					if scoped.Passed {
						status = "passed"
						message = "登记测试集通过"
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
				verified := false
				for _, entry := range e2e.Selected {
					if entry.ID == id && entry.Env == d.Environment && entry.Level == d.Level {
						var groupEntries []e2eScenario
						for _, selected := range e2e.Selected {
							if selected.Package == entry.Package {
								groupEntries = append(groupEntries, selected)
							}
						}
						for _, group := range e2e.Groups {
							if !group.Summary.Complete {
								continue
							}
							for _, test := range group.Summary.Tests {
								if test.Package == strings.TrimPrefix(entry.Package, "./") || strings.HasSuffix(test.Package, strings.TrimPrefix(entry.Package, ".")) {
									if test.Name == entry.Test && (test.Status == "pass" || test.Status == "fail") {
										matched = test.Status
										verified = checkE2EGroup(group, groupEntries, opts.out) == nil
									}
								}
							}
						}
					}
				}
				if e == nil && json.Unmarshal(body, &evidence) == nil && evidence.ID == id && evidence.Cleanup && matched != "" {
					status = "failed"
					message = "场景断言失败"
					if matched == "pass" {
						status, message = "incomplete", "场景证据、执行集合或清理核验不完整"
						if verified && evidence.Passed && e2e.Cleanup {
							status, message = "passed", "所选场景与清理通过"
						}
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
		if status != "passed" {
			failures = append(failures, fmt.Errorf("%s: %s", id, message))
		}
		report := testmanagement.Report{ID: run.ids[id], StartedAt: run.started, FinishedAt: time.Now().UTC(), Definition: d, Command: run.command, Repository: run.repository, Branch: run.branch, Evidence: tm.ReportEvidence{Path: "summary.json", Bytes: len(summaryRaw), SHA256: testmanagement.ContentDigest(summaryRaw)}, Sequence: sequence, Selection: []string{id}, Environment: actualEnvironments[id], Definitions: map[string]string{id: testmanagement.Digest(d)}, BeforeCommit: before.commit, AfterCommit: after.commit, BeforeClean: before.clean, AfterClean: after.clean, Results: []testmanagement.Result{{TestID: id, Status: status, Message: message, Log: "summary.json"}}}
		path := filepath.Join(opts.out, "managed-"+id+".json")
		if e := testmanagement.WriteRecordJSON(path, report); e != nil {
			fmt.Fprintln(os.Stderr, "write registered report:", e)
			failures = append(failures, e)
			if resetErr := runtimeprogress.Update(context.Background(), map[string]bool{testmanagement.ProgressKey(id): false}, true); resetErr != nil {
				failures = append(failures, resetErr)
			}
			continue
		}
		// Publish completion only after its evidence is durable.
		if e := runtimeprogress.Update(context.Background(), map[string]bool{testmanagement.ProgressKey(id): status == "passed"}, true); e != nil {
			failures = append(failures, e)
			report.Results[0].Status, report.Results[0].Message = "incomplete", "运行时进度写入失败："+e.Error()
			if rewriteErr := testmanagement.WriteRecordJSON(path, report); rewriteErr != nil {
				failures = append(failures, rewriteErr)
			}
		}
	}
	fmt.Fprintln(os.Stderr, "测试报告已保存；可用 test-feedback record -report-dir <目录> 写入 Git 历史文件。")
	return errors.Join(failures...)
}

func managedReportSequence() uint64 { return uint64(time.Now().UnixNano()) }
