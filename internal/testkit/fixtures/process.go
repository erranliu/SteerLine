package fixtures

import (
	"encoding/json"
	"fmt"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"os/exec"
	"time"
)

// PRState is the isolated PR-provider state used by native workflow tests.
type PRState struct {
	PRHeadRevision string
	PRState        string
	PRMerged       bool
	PRMergedAt     *time.Time
	PRMergeCommit  string
}

func ReadPRState(path string) PRState {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var db PRState
	if err = json.Unmarshal(data, &db); err != nil {
		panic(err)
	}
	return db
}

func WritePRState(path string, db PRState) {
	data, err := json.Marshal(db)
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		panic(err)
	}
}

func FakeMain(args []string, publish func(string, string) error) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "publication-git":
		if len(args) > 3 && args[2] == "remote" && args[3] == "get-url" {
			fmt.Println("https://github.com/owner/repo.git")
			return 0
		}
		cmd := exec.Command(args[1], args[2:]...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				return e.ExitCode()
			}
			return 1
		}
		return 0
	case "git-proxy":
		gitArgs := append([]string(nil), args[2:]...)
		if len(gitArgs) > 0 && gitArgs[0] == "fetch" {
			if remote := os.Getenv("STEERLINE_FAKE_GIT_FETCH_REMOTE"); remote != "" {
				for i := 1; i < len(gitArgs); i++ {
					if gitArgs[i] == "origin" {
						gitArgs[i] = remote
						break
					}
				}
			}
		}
		cmd := exec.Command(args[1], gitArgs...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "publish-control":
		if len(args) != 3 {
			return 2
		}
		err := publish(args[1], args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "pr-query":
		db := ReadPRState(args[1])
		state := db.PRState
		if state == "" {
			state = "open"
		}
		info := executionmodel.PRInfo{URL: args[2], Repository: args[3], BaseBranch: "main", HeadRevision: db.PRHeadRevision, State: state, Merged: db.PRMerged, MergedAt: db.PRMergedAt}
		if db.PRMergeCommit != "" {
			info.MergeCommit = &db.PRMergeCommit
		}
		_ = json.NewEncoder(os.Stdout).Encode(info)
		return 0
	case "pr-close":
		db := ReadPRState(args[1])
		db.PRState, db.PRMerged = "closed_unmerged", false
		db.PRMergedAt, db.PRMergeCommit = nil, ""
		WritePRState(args[1], db)
		info := executionmodel.PRInfo{URL: args[2], Repository: args[3], BaseBranch: "main", HeadRevision: db.PRHeadRevision, State: "closed_unmerged"}
		_ = json.NewEncoder(os.Stdout).Encode(info)
		return 0
	case "sleep":
		time.Sleep(time.Hour)
		return 0
	case "spawn-owned-child":
		if len(args) != 2 {
			return 2
		}
		child := exec.Command(os.Args[0], "--fake", "sleep")
		if err := child.Start(); err != nil {
			return 1
		}
		if err := os.WriteFile(args[1], []byte(fmt.Sprint(child.Process.Pid)), 0600); err != nil {
			_ = child.Process.Kill()
			return 1
		}
		time.Sleep(time.Hour)
		return 0
	}
	return 2
}
