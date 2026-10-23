package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/pathguard"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func recordReports(args []string) int {
	flags := flag.NewFlagSet("test-feedback record", flag.ContinueOnError)
	dir := flags.String("report-dir", "", "absolute directory from one complete test execution")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*dir) {
		fmt.Fprintln(os.Stderr, "record requires -report-dir <absolute report directory>")
		return 2
	}
	name, err := exportReports(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(name)
	return 0
}

// This is a repository-side writer; it never opens an application or database.
func exportReports(dir string) (string, error) {
	root := managedGitValue("rev-parse", "--show-toplevel")
	common := managedGitValue("rev-parse", "--path-format=absolute", "--git-common-dir")
	if root == "" || common == "" {
		return "", errors.New("record must run inside the tested Git repository")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var reports []testmanagement.Report
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "managed-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := filepath.Join(dir, entry.Name())
		raw, e := readReportFile(name)
		if e != nil {
			return "", e
		}
		report, e := testmanagement.ParseReport(raw)
		if e != nil {
			return "", fmt.Errorf("%s: %w", name, e)
		}
		if !strings.EqualFold(filepath.Clean(report.Repository), filepath.Clean(common)) {
			return "", errors.New("report belongs to a different Git repository")
		}
		reports = append(reports, report)
	}
	commit, run, err := testmanagement.ExportRun(reports)
	if err != nil {
		return "", err
	}
	targetDir := filepath.Join(root, filepath.FromSlash(testmanagement.RecordsPath))
	// Reject redirected output directories instead of writing outside the repository.
	for current := targetDir; current != filepath.Dir(root) && current != root; current = filepath.Dir(current) {
		info, e := os.Lstat(current)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("record output directory must be a normal repository directory")
		}
	}
	if err = os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}
	// Fail concurrent writers explicitly; never silently lose a run.
	lock, err := os.OpenFile(filepath.Join(targetDir, ".write.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("history writer already active or lock unavailable: %w", err)
	}
	defer os.Remove(lock.Name())
	defer lock.Close()
	target := filepath.Join(targetDir, commit+".json")
	file := testmanagement.RecordFile{}
	if raw, e := readHistoryFile(target); e == nil {
		file, err = testmanagement.ParseRecordFile(raw)
		if err != nil {
			return "", err
		}
	} else if !os.IsNotExist(e) {
		return "", e
	}
	changed, err := file.Append(run)
	if err != nil {
		return "", err
	}
	if changed {
		err = testmanagement.WriteRecordJSON(target, file)
	}
	return target, err
}
func readReportFile(name string) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Size() > 128<<10 {
		return nil, errors.New("managed report exceeds 128 KiB")
	}
	return readHistoryFile(name)
}
func readHistoryFile(name string) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("history/report must be a regular file")
	}
	if err = pathguard.RejectHardLinkedFile(name); err != nil {
		return nil, err
	}
	return os.ReadFile(name)
}
