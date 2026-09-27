package viewer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

// TestRealSupervisedViewerDecisionRender measures the real SQLite-to-viewer
// projection during an authorized remote supervised prepare probe.
func TestRealSupervisedViewerDecisionRender(t *testing.T) {
	root := os.Getenv("STEERLINE_REAL_VIEWER_PROBE_ROOT")
	if root == "" {
		t.Skip("set STEERLINE_REAL_VIEWER_PROBE_ROOT during a live supervised Run")
	}
	c := steerline.Config{Repository: filepath.Join(root, "local-identity"), Remote: "origin", Branch: "main", RunDir: filepath.Join(root, "runtime"), RuntimeFile: "runtime.json", Heartbeat: "5s", Git: steerline.Command{Executable: "git"}}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := NewFileSource(c)
	started := time.Now().UTC()
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("live remote decision did not reach the viewer source")
		default:
		}
		var payload []byte
		err = db.QueryRow("SELECT payload FROM decisions ORDER BY rowid LIMIT 1").Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var record steerline.DecisionRecord
		if err = json.Unmarshal(payload, &record); err != nil {
			t.Fatal(err)
		}
		var request steerline.DecisionRequest
		if err = json.Unmarshal(record.Request, &request); err != nil {
			t.Fatal(err)
		}
		readStarted := time.Now()
		observation := source.Snapshot()
		if observation.Runtime == nil || len(observation.Runtime.Runs) == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var run *steerline.Run
		for _, candidate := range observation.Runtime.Runs {
			if candidate.Decision != nil && candidate.Decision.ID == record.DecisionID {
				run = candidate
				break
			}
		}
		if run == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		model := NewModel(c, source, Options{RunID: run.RunID})
		model.width, model.height, model.detailTab = 160, 50, 3
		updated, _ := model.Update(runsMsg{observation: observation})
		rendered := updated.(Model).View()
		if !strings.Contains(rendered, request.Question) {
			t.Fatalf("real viewer did not render the original remote question %q", request.Question)
		}
		readRender := time.Since(readStarted)
		centerToRender := time.Since(record.CreatedAt)
		if record.CreatedAt.Before(started) {
			t.Logf("decision predated probe start by %s; rendering was verified without an arrival-latency claim", started.Sub(record.CreatedAt))
		} else if centerToRender > 2*time.Second {
			t.Fatalf("real center decision to viewer render latency %s exceeded 2s", centerToRender)
		}
		if evidencePath := os.Getenv("STEERLINE_REAL_VIEWER_PROBE_EVIDENCE_PATH"); evidencePath != "" {
			evidence := map[string]any{"captured_at": time.Now().UTC(), "source": "live supervised SQLite projection", "run_id": run.RunID, "decision_id": record.DecisionID, "question_rendered": true, "decision_created_at": record.CreatedAt, "probe_started_at": started, "center_to_render_ms": centerToRender.Milliseconds(), "snapshot_and_render_ms": readRender.Milliseconds(), "viewer_refresh_interval_ms": 700, "arrival_latency_measured": !record.CreatedAt.Before(started), "view_width": model.width, "view_height": model.height}
			encoded, marshalErr := json.MarshalIndent(evidence, "", "  ")
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if err = os.MkdirAll(filepath.Dir(evidencePath), 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(evidencePath, append(encoded, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("remote question rendered in viewer: center-to-render=%s source-read-and-render=%s", centerToRender, readRender)
		return
	}
}

// TestRealSupervisedViewerPublishRender starts before a real remote publish
// result and measures its committed center projection through the viewer.
func TestRealSupervisedViewerPublishRender(t *testing.T) {
	root := os.Getenv("STEERLINE_REAL_VIEWER_PROBE_ROOT")
	if root == "" {
		t.Skip("set STEERLINE_REAL_VIEWER_PROBE_ROOT during a live supervised Run")
	}
	c := steerline.Config{Repository: filepath.Join(root, "local-identity"), Remote: "origin", Branch: "main", RunDir: filepath.Join(root, "runtime"), RuntimeFile: "runtime.json", Heartbeat: "5s", Git: steerline.Command{Executable: "git"}}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := NewFileSource(c)
	started := time.Now().UTC()
	deadline := time.NewTimer(35 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("real remote publish did not reach the viewer source")
		default:
		}
		var payload []byte
		err = db.QueryRow("SELECT payload FROM step_attempts WHERE step_id='publish_pr' ORDER BY attempt_number DESC LIMIT 1").Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var attempt steerline.StepAttemptRecord
		if err = json.Unmarshal(payload, &attempt); err != nil {
			t.Fatal(err)
		}
		if attempt.Status != "succeeded" {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		readStarted := time.Now()
		observation := source.Snapshot()
		if observation.Runtime == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var run *steerline.Run
		for _, candidate := range observation.Runtime.Runs {
			if candidate.PR.URL != nil && *candidate.PR.URL != "" {
				run = candidate
				break
			}
		}
		if run == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		model := NewModel(c, source, Options{RunID: run.RunID})
		model.width, model.height, model.detailTab = 160, 50, 2
		updated, _ := model.Update(runsMsg{observation: observation})
		rendered := updated.(Model).View()
		if !strings.Contains(rendered, *run.PR.URL) {
			t.Fatalf("real viewer did not render the original remote PR URL %q", *run.PR.URL)
		}
		readRender := time.Since(readStarted)
		centerToRender := time.Since(attempt.UpdatedAt)
		if attempt.UpdatedAt.Before(started) {
			t.Logf("publish result predated probe start by %s; rendering was verified without an arrival-latency claim", started.Sub(attempt.UpdatedAt))
		} else if centerToRender > 2*time.Second {
			t.Fatalf("real center publish result to viewer render latency %s exceeded 2s", centerToRender)
		}
		if evidencePath := os.Getenv("STEERLINE_REAL_VIEWER_PUBLISH_EVIDENCE_PATH"); evidencePath != "" {
			evidence := map[string]any{"captured_at": time.Now().UTC(), "source": "live supervised SQLite projection", "run_id": run.RunID, "publish_attempt_id": attempt.AttemptID, "pr_url_rendered": *run.PR.URL, "center_result_updated_at": attempt.UpdatedAt, "probe_started_at": started, "center_to_render_ms": centerToRender.Milliseconds(), "snapshot_and_render_ms": readRender.Milliseconds(), "viewer_refresh_interval_ms": 700, "arrival_latency_measured": !attempt.UpdatedAt.Before(started), "view_width": model.width, "view_height": model.height}
			encoded, marshalErr := json.MarshalIndent(evidence, "", "  ")
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if err = os.MkdirAll(filepath.Dir(evidencePath), 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(evidencePath, append(encoded, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("remote PR rendered in viewer: center-to-render=%s source-read-and-render=%s", centerToRender, readRender)
		return
	}
}
