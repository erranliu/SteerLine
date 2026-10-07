package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/console"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func TestCandidateConsoleWiring(t *testing.T) {
	// Five original native fixture processes plus their control and UI reads
	// need a bounded shared budget, including under the standard Full load.
	timeout := 8 * time.Minute
	if os.Getenv("STEERLINE_CC5_MANIFEST") != "" {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	f := newLifecycleProject(t, "local")
	var profiles []any
	for i := 0; i < 6; i++ {
		payload, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: os.Args[0], Model: fmt.Sprintf("fixture-model-%d", i)})
		require(t, err)
		profiles = append(profiles, map[string]any{"id": fmt.Sprintf("p%d", i), "name": fmt.Sprintf("隔离方案%d", i), "host": "local", "agent": "codex", "implementation_configs": []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: payload}}})
	}
	require(t, os.WriteFile(steerlineapp.GlobalExecutionConfigPath(), mustJSON(map[string]any{"version": 2, "default_execution_profile": "p0", "execution_profiles": profiles}), 0600))
	var err error
	f.config, err = steerlineapp.LoadConfig(f.configPath)
	require(t, err)
	createIntegrationTask(t, ctx, f, 901)
	svc := startIntegrationService(t, ctx, f.config, &integrationLifecycleResolver{})
	defer svc.stop(t)
	client, err := steerlineapp.OpenClient(ctx, f.configPath)
	require(t, err)
	defer client.Close()
	ref := client.Project()
	m, err := console.NewModel(ctx, []productapi.ProjectRef{ref}, console.Options{Clients: map[string]console.ConsoleClient{ref.Key: client}, ApplicationHome: steerlineapp.HomeDir()})
	require(t, err)
	defer m.Close()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 80})
	m = next.(console.Model)
	var execute func(tea.Cmd, int)
	execute = func(cmd tea.Cmd, depth int) {
		t.Helper()
		if cmd == nil {
			return
		}
		if depth > 32 {
			t.Fatal("console command chain did not settle")
		}
		message := cmd()
		if batch, ok := message.(tea.BatchMsg); ok {
			for _, child := range batch {
				execute(child, depth+1)
			}
			return
		}
		next, followup := m.Update(message)
		m = next.(console.Model)
		execute(followup, depth+1)
	}
	step := func(key tea.KeyMsg) {
		t.Helper()
		next, cmd := m.Update(key)
		m = next.(console.Model)
		execute(cmd, 0)
	}
	key := func(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }
	step(key('r'))
	step(key('s'))
	if !containsConsoleText(m.View(), "选择执行方案") {
		t.Fatal("selected ready task did not open its execution profile choice", m.View())
	}
	step(tea.KeyMsg{Type: tea.KeyEnter})
	t.Logf("first explicit start UI: %s", m.View())
	var primary *productapi.CandidateSummary
	// Native fixture startup can exceed 30 seconds under Full's parallel load.
	waitFor(t, time.Minute, func() bool {
		snap := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
		if snap.Observation.Page == nil {
			return false
		}
		for _, r := range snap.Observation.Page.Runs {
			if r.Decision != nil && r.SessionID != "" {
				primary = r
				return true
			}
		}
		return false
	})
	step(key('r'))
	step(key('b'))
	step(tea.KeyMsg{Type: tea.KeyDown})
	step(tea.KeyMsg{Type: tea.KeyEnter})
	t.Logf("second explicit start UI: %s", m.View())
	// The UI's archived request is read through the public control client.
	var candidateRequest *productapi.ControlRequest
	waitFor(t, time.Minute, func() bool {
		snap := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
		if snap.Observation.Page == nil {
			return false
		}
		for _, r := range snap.Observation.Page.Runs {
			if r.RunID == primary.RunID || r.SessionID == "" {
				continue
			}
			req, e := client.Request(ctx, r.RunID)
			if e == nil {
				candidateRequest = req
				return true
			}
		}
		return false
	})
	var p productapi.ControlPayload
	require(t, json.Unmarshal(candidateRequest.Payload, &p))
	if p.TaskID != "901" || p.ExecutionProfileID != "p1" || p.ExpectedTaskVersion == 0 || p.ExpectedGraphRevision == 0 {
		t.Fatalf("console start lost fixed identity: %+v", p)
	}
	r, err := client.WaitControl(ctx, candidateRequest.RequestID, 100*time.Millisecond)
	require(t, err)
	if r.Status != "applied" {
		t.Fatal(r)
	}
	revision := candidateRequest.ExpectedControlRevision
	_, err = client.SubmitControl(ctx, productapi.ControlInput{Action: productapi.StartCandidate, Payload: p, RequestID: candidateRequest.RequestID, RequestedBy: candidateRequest.RequestedBy, InstanceID: candidateRequest.InstanceID, ExpectedRevision: &revision})
	require(t, err)
	t.Logf("UI start request %s: %s; original receipt: %s", candidateRequest.RequestID, candidateRequest.Payload, mustJSON(r))
	for i := 2; i < 5; i++ {
		snap := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
		page, err := client.ShowTask(ctx, "901")
		require(t, err)
		profile := ref.Presentation.Profiles[i]
		revision := snap.Connection.Snapshot.Page.Control.Revision
		in := productapi.ControlInput{Action: productapi.StartCandidate, RequestID: newID(), InstanceID: svc.instanceID, ExpectedRevision: &revision, Payload: productapi.ControlPayload{TaskID: "901", ExpectedTaskVersion: page.Tasks[0].Version, ExpectedGraphRevision: page.GraphRevision, ExecutionProfileID: profile.ID, ExecutionProfileDigest: profile.Digest}}
		_, err = client.SubmitControl(ctx, in)
		require(t, err)
		receipt, err := client.WaitControl(ctx, in.RequestID, 100*time.Millisecond)
		require(t, err)
		if receipt.Status != "applied" {
			t.Fatal(receipt)
		}
	}
	step(key('r'))
	step(key('b'))
	if !containsConsoleText(m.View(), "已达5个") {
		t.Fatal("UI did not show start-time participation limit", m.View())
	}
	limitSnap := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
	limitPage, err := client.ShowTask(ctx, "901")
	require(t, err)
	limitRevision := limitSnap.Connection.Snapshot.Page.Control.Revision
	limit := productapi.ControlInput{Action: productapi.StartCandidate, RequestID: newID(), InstanceID: svc.instanceID, ExpectedRevision: &limitRevision, Payload: productapi.ControlPayload{TaskID: "901", ExpectedTaskVersion: limitPage.Tasks[0].Version, ExpectedGraphRevision: limitPage.GraphRevision, ExecutionProfileID: ref.Presentation.Profiles[5].ID, ExecutionProfileDigest: ref.Presentation.Profiles[5].Digest}}
	_, err = client.SubmitControl(ctx, limit)
	require(t, err)
	limited, err := client.WaitControl(ctx, limit.RequestID, 100*time.Millisecond)
	require(t, err)
	if limited.Status != "rejected" || !strings.Contains(limited.Message, "candidate_limit_reached") {
		t.Fatalf("serial sixth start was not rejected: %+v", limited)
	}
	t.Logf("real start-time participation receipt: %s", mustJSON(limited))
	if manifest := os.Getenv("STEERLINE_CC5_MANIFEST"); manifest != "" {
		// Opt-in native console preparation only. The human supplies actual IME
		// and window operations; neither fake KeyMsgs above nor this hold pass CC5.
		cli := buildProductLifecycleCLI(t)
		require(t, os.MkdirAll(filepath.Dir(manifest), 0700))
		require(t, os.WriteFile(manifest, mustJSON(map[string]any{"status": "ready_for_manual_CC5_not_passed", "home": steerlineapp.HomeDir(), "config": f.configPath, "executable": cli, "repository": f.repository, "instance": svc.instanceID, "worker": "isolated controlled app-server test binary", "stop_file": manifest + ".stop", "environment": map[string]string{
			"STEERLINE_TEST_REMOTE_URL":          f.remoteURL,
			"STEERLINE_FAKE_GIT_FETCH_REMOTE":    f.bareRemote,
			"STEERLINE_INTEGRATION_HISTORY_FILE": os.Getenv("STEERLINE_INTEGRATION_HISTORY_FILE"),
		}}), 0600))
		t.Logf("CC5 isolated native fixture ready: %s; awaiting explicit stop file", manifest)
		for {
			if _, err := os.Stat(manifest + ".stop"); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("CC5 manual fixture timed out; no native pass recorded")
			case <-time.After(time.Second):
			}
		}
		return
	}
	// Feedback is submitted through the actual console against the selected original Decision.
	step(key('r'))
	step(key('a'))
	if !containsConsoleText(m.View(), "提交预览") || !containsConsoleText(m.View(), "你的答复") {
		t.Fatalf("original decision draft did not open: %s", m.View())
	}
	step(tea.KeyMsg{Type: tea.KeyTab})
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("接线验证：精确原候选")})
	for attempt := 0; attempt < 3; attempt++ {
		step(tea.KeyMsg{Type: tea.KeyF2})
		if !strings.Contains(m.View(), "stale_instance: SQLite projection does not match the ready service instance") {
			break
		}
		// This API rejection precedes command admission. Simulate a human
		// refreshing and explicitly resubmitting the same unsent draft; never
		// retry unknown outcomes or a changed worker/question/turn.
		waitFor(t, 30*time.Second, func() bool {
			diagnostic := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
			if diagnostic.Observation.Page == nil {
				return false
			}
			rt := diagnostic.Observation.Page
			if rt.Service.InstanceID != svc.instanceID {
				t.Fatal("known rejection changed the original service instance")
			}
			for _, current := range rt.Runs {
				if current.RunID == primary.RunID {
					if current.ExecutionID != primary.ExecutionID || current.SessionID != primary.SessionID || current.TurnNumber != primary.TurnNumber || current.Decision == nil || current.Decision.ID != primary.Decision.ID || current.Decision.Status != "pending" {
						t.Fatal("known rejection changed or consumed the original question/worker/turn")
					}
					return rt.Service.Ready && rt.Service.Status == "running"
				}
			}
			return false
		})
		t.Log("original pre-admission stale-instance rejection; exact unsent question/worker/turn retained; explicit UI refresh and resubmit")
		step(tea.KeyMsg{Type: tea.KeyEsc})
		step(key('r'))
		step(key('a'))
		if !containsConsoleText(m.View(), "提交预览") || !containsConsoleText(m.View(), "你的答复") || !containsConsoleText(m.View(), "接线验证：精确原候选") {
			t.Fatal("explicit refresh lost the original unsent draft", m.View())
		}
	}
	t.Logf("feedback submission UI: %s", m.View())
	if strings.Contains(m.View(), "stale_revision") || strings.Contains(m.View(), "stale_instance") || strings.Contains(m.View(), "答案命令结果待确认") {
		diagnostic := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
		if diagnostic.Connection.Snapshot.Page != nil {
			t.Logf("feedback connection service: %s", mustJSON(diagnostic.Connection.Snapshot.Page.Service))
		}
		if diagnostic.Observation.Page != nil {
			t.Logf("feedback canonical service: %s", mustJSON(diagnostic.Observation.Page.Service))
		}
		t.Fatal("feedback did not reach its exact original identity", m.View())
	}
	waitFor(t, time.Minute, func() bool {
		snap := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
		for _, r := range snap.Observation.Page.Runs {
			if r.RunID == primary.RunID && r.TurnNumber >= 2 {
				return true
			}
		}
		return false
	})
	step(key('r'))
	step(key('z'))
	step(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, 30*time.Second, func() bool {
		snap := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
		for _, r := range snap.Observation.Page.Runs {
			if r.RunID == primary.RunID {
				return r.Abandonment != nil
			}
		}
		return false
	})
	step(key('r'))
	step(key('d')) // Enter now opens interaction/session; d owns full history facts.
	if !containsConsoleText(m.View(), "已放弃") {
		t.Fatal("UI lost original exit/stop facts", m.View())
	}
	snap := client.ReadProject(ctx, "901", productapi.ProjectSnapshot{})
	t.Logf("canonical five-candidate snapshot after exact feedback/exit: %s", mustJSON(snap.Observation.Page.Runs))
	if len(snap.Observation.Page.Runs) != 5 {
		t.Fatal("duplicate request created another candidate")
	}
	// A sixth start is rejected by the real service before exit (the UI test above
	// exercises its display); merged-PR authoritative guard is covered below.
	t.Run("merged_PR_service_guard", func(t *testing.T) { testCandidateConsoleMergedGuard(t) })
}

func containsConsoleText(text, want string) bool { return strings.Contains(text, want) }

func testCandidateConsoleMergedGuard(t *testing.T) {
	// Retain the existing real application/provider delivery path rather than
	// manufacturing a merged flag in the business database.
	testAdditionalImplementationWithCandidateGuard(t, true)
}
