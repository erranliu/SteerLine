package viewer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/erranliu/SteerLine/internal/steerline"
)

func longOverflowRun() *steerline.Run {
	long := "Workspace recovery check failed: GetFileAttributesEx C:/SteerlineRuntime/merge-h-308-code/worktrees/9f18fd1a874c4da794212c149ce7cce5: The system cannot find the file specified.; no recreation attempted; " + strings.Repeat("recovery30 can ", 30)
	last := strings.Repeat("prepare/started worktree create ", 30)
	return &steerline.Run{
		TaskID: "n308-26", RunID: "r26", Title: "M8a Jigsaw",
		ExecutionStatus: "failed", FinishTaskStatus: "blocked",
		Error: &long, LastEvent: &last,
		LogPath: strptr("log-overflow"),
	}
}

func TestViewerLinesFitPaneWidth(t *testing.T) {
	prio := 2
	f := &fakeSource{
		runs:       []*steerline.Run{longOverflowRun()},
		taskDetail: map[string]steerline.TaskDetail{"n308-26": {ID: "n308-26", Type: "bug", Status: "open", Priority: &prio, Description: strings.Repeat("desc ", 300)}},
		activity: map[string][]steerline.ActivityEvent{
			"log-overflow": {
				{Kind: "command", Status: "completed", Title: strings.Repeat("averylongcommand", 20), Detail: strings.Repeat("out ", 200), ItemID: "c1"},
				{Kind: "error", Status: "failed", Title: strings.Repeat("longerror", 20), Detail: strings.Repeat("e ", 200)},
			},
		},
	}
	for _, width := range []int{80, 120, 170} {
		m := NewModel(steerline.Config{}, f, Options{})
		m.width, m.height = width, 52
		rt, _ := f.Snapshot()
		mm, _ := m.Update(runsMsg{runs: f.runs, rt: rt})
		m = mm.(Model)
		mm, _ = m.Update(activityMsg{logPath: "log-overflow", events: f.activity["log-overflow"], from: 2})
		m = mm.(Model)
		leftW := m.width / 3
		if leftW < 30 {
			leftW = 30
		}
		if leftW > 50 {
			leftW = 50
		}
		rightW := m.width - leftW - 1
		check := func(name string, lines []string, limit int) {
			for i, ln := range lines {
				if strings.Contains(ln, "\n") {
					t.Errorf("w=%d %s[%d] contains newline: %q", width, name, i, ln)
				}
				if wd := lipgloss.Width(ln); wd > limit {
					t.Errorf("w=%d %s[%d] is %d wide (limit %d): %q", width, name, i, wd, limit, ln)
				}
			}
		}
		check("runs", m.viewRuns(leftW, 49), leftW)
		check("right", m.viewRight(rightW, 49), rightW)
	}
}
