package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/testmanagement"
)

type managedSource struct {
	commit string
	clean  bool
}

func observeManagedSource() managedSource {
	commit := strings.TrimSpace(commandOutput("git", "rev-parse", "HEAD"))
	status, err := exec.Command("git", "status", "--porcelain", "--untracked-files=normal").Output()
	return managedSource{commit: commit, clean: err == nil && (len(commit) == 40 || len(commit) == 64) && strings.TrimSpace(string(status)) == ""}
}

// Existing execution and exit codes remain authoritative. This only emits a
// small registered report beside the original summary/logs, even after failure.
func writeManagedReports(opts options, before managedSource, sequence uint64) {
	if opts.level == "" || opts.buildOnly || opts.summarizeLog != "" || len(opts.goTestArgs) > 0 {
		return
	}
	root, err := e2eRoot()
	if err != nil {
		return
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(testmanagement.CataloguePath)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "registered tests: catalogue unavailable:", err)
		return
	}
	catalog, err := testmanagement.ParseCatalogue(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "registered tests:", err)
		return
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
			return
		}
		for _, entry := range scenarios {
			selected = append(selected, entry.ID)
			actualEnvironments[entry.ID] = entry.Env
		}
	}
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
		if d.Environment != actualEnvironments[id] {
			status, message = "incomplete", "登记环境与实际选择不一致"
		}
		report := testmanagement.Report{ID: fmt.Sprintf("%d-%s", sequence, id), Sequence: sequence, Selection: []string{id}, Environment: actualEnvironments[id], Definitions: map[string]string{id: testmanagement.Digest(d)}, BeforeCommit: before.commit, AfterCommit: after.commit, BeforeClean: before.clean, AfterClean: after.clean, Results: []testmanagement.Result{{TestID: id, Status: status, Message: message, Log: "summary.json"}}}
		if e := publishManagedReport(filepath.Join(opts.out, "managed-"+id+".json"), report); e != nil {
			fmt.Fprintln(os.Stderr, "write registered report:", e)
		}
	}
}
func managedReportSequence() uint64 { return uint64(time.Now().UnixNano()) }

// The managed report is the completion marker; publish it after summary/logs.
func publishManagedReport(target string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".managed-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(raw, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), target)
}
