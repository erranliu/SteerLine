package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type projectLaunchRun struct {
	mv.Attempt
	Entry             string `json:"entry_id"`
	Name              string `json:"name"`
	PR                int    `json:"pr_number,omitempty"`
	CleanupDiagnostic string `json:"cleanup_diagnostic,omitempty"`
}

var projectLaunchRunID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type projectLaunchView struct {
	Path       string             `json:"path"`
	Branch     string             `json:"branch"`
	Head       string             `json:"head"`
	Ready      bool               `json:"ready"`
	Diagnostic string             `json:"diagnostic,omitempty"`
	Entries    []mv.ProjectLaunch `json:"entries"`
	Run        *projectLaunchRun  `json:"run,omitempty"`
}

func (s *manualVerification) projectLaunchDir() string {
	return filepath.Join(s.config.RunDir, "manual-verification", "project")
}

func (s *manualVerification) prLaunchDir() string {
	return filepath.Join(s.config.RunDir, "manual-verification", "pr")
}

func (s *manualVerification) saveProjectLaunch(run projectLaunchRun) error {
	dir := s.projectLaunchDir()
	if run.PR > 0 {
		return s.savePRRun(run)
	}
	if err := ordinaryVerificationPath(s.config.RunDir, dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	if err := host.AtomicWrite(filepath.Join(dir, "latest.json"), raw); err != nil {
		return err
	}
	s.projectRun = &run
	return nil
}

func (s *manualVerification) recoverProjectLaunch(m *projectMaintenance) error {
	name := filepath.Join(s.projectLaunchDir(), "latest.json")
	if err := ordinaryVerificationPath(s.config.RunDir, name); err != nil {
		return err
	}
	raw, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var run projectLaunchRun
	if err := json.Unmarshal(raw, &run); err != nil {
		return err
	}
	if !projectLaunchRunID.MatchString(run.ID) || !taskmodel.ValidCommit(run.Head) {
		return errors.New("无效的项目启动记录")
	}
	if run.Active() {
		if verificationTreeAbsent(run.Attempt) {
			run.Status = "stopped"
			run.Diagnostic = "宿主重启后已核对原启动"
		} else {
			run.Status = "unknown"
			run.Diagnostic = "原启动进程尚未核实"
			m.checkoutMu.Lock()
		}
		return s.saveProjectLaunch(run)
	}
	s.projectRun = &run
	return nil
}

func projectLaunchEntries(ctx context.Context, m *projectMaintenance, head string) ([]mv.ProjectLaunch, error) {
	raw, err := m.readCommittedMaterial(ctx, head, mv.ProjectLaunchPath)
	if err == nil {
		var entries []mv.ProjectLaunch
		entries, err = mv.DecodeProjectLaunches(raw)
		if err == nil {
			return entries, nil
		}
	}
	return nil, fmt.Errorf("%s: %w", mv.ProjectLaunchPath, err)
}

func (s *manualVerification) projectLaunchView(ctx context.Context, m *projectMaintenance) (projectLaunchView, error) {
	view := projectLaunchView{Path: m.config.Repository, Branch: m.config.Branch, Entries: []mv.ProjectLaunch{}}
	s.mu.Lock()
	if s.projectRun != nil {
		run := *s.projectRun
		run.Process = nil
		view.Run = &run
	}
	s.mu.Unlock()
	state, err := m.readinessState(ctx)
	if err != nil {
		return view, err
	}
	view.Head = state.Local.Commit
	if taskmodel.ValidCommit(view.Head) {
		entries, err := projectLaunchEntries(ctx, m, view.Head)
		if err != nil {
			view.Diagnostic = err.Error()
		} else {
			view.Entries = entries
		}
	}
	if err := requireProjectReadiness(state.Readiness); err != nil {
		view.Diagnostic = strings.TrimSpace(err.Error() + " " + view.Diagnostic)
	} else {
		view.Ready = len(view.Entries) != 0
	}
	return view, nil
}

func (s *manualVerification) startProjectLaunch(ctx context.Context, m *projectMaintenance, id, head, entryID string) error {
	if !m.checkoutMu.TryLock() {
		return errors.New("纳管目录正在用于项目启动或同步")
	}
	started := false
	defer func() {
		if !started {
			m.checkoutMu.Unlock()
		}
	}()
	view, err := s.projectLaunchView(ctx, m)
	if err != nil {
		return err
	}
	if !view.Ready {
		return errors.New(view.Diagnostic)
	}
	if view.Head != head {
		return errors.New("纳管目录提交已变化，请刷新入口后重试")
	}
	for _, entry := range view.Entries {
		if entry.ID != entryID {
			continue
		}
		run := projectLaunchRun{Attempt: mv.Attempt{ID: id, Head: head, Status: "preparing"}, Entry: entry.ID, Name: entry.Name}
		logDir := filepath.Join(s.projectLaunchDir(), id)
		if err := ordinaryVerificationPath(s.config.RunDir, logDir); err != nil {
			return err
		}
		if err := m.reserveBaseline(head, func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.projectRun != nil && s.projectRun.Active() {
				return errors.New("原项目启动尚未停止")
			}
			return s.saveProjectLaunch(run)
		}); err != nil {
			return err
		}
		launchCtx, cancel := context.WithCancel(s.ctx)
		s.mu.Lock()
		s.stops[id] = cancel
		s.mu.Unlock()
		started = true
		s.workers.Go(func() {
			defer cancel()
			patch := func(update func(*mv.Attempt)) error {
				s.mu.Lock()
				defer s.mu.Unlock()
				update(&run.Attempt)
				err := s.saveProjectLaunch(run)
				// Keep the live result observable even if the durable copy fails.
				if err != nil {
					run.Diagnostic = strings.TrimSpace(run.Diagnostic + " 保存启动状态失败：" + err.Error())
					current := run
					s.projectRun = &current
				}
				return err
			}
			err := os.Mkdir(logDir, 0700)
			if err == nil {
				// Recheck the actual checkout immediately before executing files from it.
				fresh := m.localBaseline(launchCtx)
				if fresh.Commit != head || fresh.Status != "synced" || fresh.Reason != "" || fresh.Branch != m.config.Branch {
					err = errors.New("纳管目录已变化，请重新检查项目状态")
				}
			}
			if err == nil {
				err = s.runLaunch(launchCtx, run.Attempt, entry.Launch, m.config.Repository, logDir, true, patch)
			}
			if err != nil && run.Active() && run.Status != "unknown" {
				_ = patch(func(a *mv.Attempt) {
					a.Status, a.Diagnostic = "failed", err.Error()
					if errors.Is(err, context.Canceled) {
						a.Status = "stopped"
					}
				})
			}
			s.mu.Lock()
			if !run.Active() {
				m.checkoutMu.Unlock()
			}
			delete(s.stops, id)
			s.mu.Unlock()
		})
		return nil
	}
	return errors.New("项目启动入口已变化，请刷新后重试")
}

func (p *runtimeProjectPort) projectLaunchCommand(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if err := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); err != nil {
		return out, err
	}
	s, m := p.manual, p.config.runtimeMaintenance
	if s == nil || m == nil || p.stopping.Load() {
		return out, projectStoppingError()
	}
	if in.TargetID != in.ProjectID || in.ExpectedVersion != 1 || in.ExpectedControlRevision != 0 || in.ExpectedGraphRevision != 0 {
		return out, errors.New("项目启动目标或版本不匹配")
	}
	var payload struct {
		Head    string `json:"head,omitempty"`
		Entry   string `json:"entry_id,omitempty"`
		Attempt string `json:"attempt_id,omitempty"`
		PR      int    `json:"pr_number,omitempty"`
	}
	if err := decodeStrictJSON(in.Payload, &payload); err != nil {
		return out, err
	}
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	raw, _ := json.Marshal(in)
	old, found, err := p.store.LookupCommand(ctx, p.config.Identity(), in.RequestID)
	if err != nil {
		return out, err
	}
	if found {
		if old.Kind != "project_launch" || !bytes.Equal(old.Payload, raw) {
			return out, errors.New("request_conflict: original project launch envelope differs")
		}
		err = p.projectLaunchOutcome(old, &out)
		return out, err
	}
	now := time.Now().UTC()
	record := ports.CommandRecord{ProjectIdentity: p.config.Identity(), CommandID: in.RequestID, Kind: "project_launch", TargetID: in.TargetID, ExpectedVersion: 1, IdempotencyKey: in.RequestID, RequestedBy: host.DefaultRequestedBy(), Payload: raw, Status: "pending", CreatedAt: now, UpdatedAt: now}
	if _, _, err = p.store.EnqueueCommand(ctx, record); err != nil {
		return out, err
	}
	switch in.Action {
	case "project_launch_pr_start":
		if !taskmodel.ValidCommit(payload.Head) || payload.Entry == "" || payload.Attempt != "" || payload.PR <= 0 {
			err = errors.New("启动需要 PR 编号、完整提交 SHA 和入口 ID")
		} else {
			err = s.startPRLaunch(ctx, m, in.RequestID, payload.PR, payload.Head, payload.Entry)
		}
	case "project_launch_pr_stop", "project_launch_pr_cleanup":
		if !projectLaunchRunID.MatchString(payload.Attempt) || payload.PR != 0 || payload.Head != "" || payload.Entry != "" {
			err = errors.New("请选择原 PR 启动记录")
		} else {
			err = s.finishPRLaunch(ctx, payload.Attempt, in.Action == "project_launch_pr_stop")
		}
	case "project_launch_start":
		if !taskmodel.ValidCommit(payload.Head) || payload.Entry == "" || payload.Attempt != "" || payload.PR != 0 {
			err = errors.New("启动需要完整提交 SHA 和入口 ID")
		} else {
			err = s.startProjectLaunch(ctx, m, in.RequestID, payload.Head, payload.Entry)
		}
	case "project_launch_stop":
		s.mu.Lock()
		if s.projectRun == nil || s.projectRun.ID != payload.Attempt || payload.Head != "" || payload.Entry != "" || payload.PR != 0 {
			err = errors.New("原项目启动已变化")
		} else if stop := s.stops[payload.Attempt]; stop != nil {
			stop()
		} else if s.projectRun.Active() {
			if verificationTreeAbsent(s.projectRun.Attempt) {
				run := *s.projectRun
				run.Status, run.Diagnostic = "stopped", ""
				err = s.saveProjectLaunch(run)
				if err == nil {
					m.checkoutMu.Unlock()
				}
			} else {
				err = errors.New("原启动进程尚未核实")
			}
		}
		s.mu.Unlock()
	default:
		err = errors.New("未知项目启动操作")
	}
	out.Status = "applied"
	if err != nil {
		out.Status, out.Code, out.Diagnostic = "rejected", "project_launch_rejected", err.Error()
	}
	record.Status = out.Status
	record.Result, _ = json.Marshal(out)
	return out, p.store.UpdateCommand(context.WithoutCancel(ctx), record)
}

func (p *runtimeProjectPort) projectLaunchReceipt(ctx context.Context, id string) (*httpcontract.CommandResponse, error) {
	if p.manual == nil {
		return nil, nil
	}
	record, found, err := p.store.LookupCommand(ctx, p.config.Identity(), id)
	if err != nil || !found || record.Kind != "project_launch" {
		return nil, err
	}
	var in httpcontract.CommandEnvelope
	if err := json.Unmarshal(record.Payload, &in); err != nil {
		return nil, err
	}
	out := maintenanceResponse(in)
	return &out, p.projectLaunchOutcome(record, &out)
}

func (p *runtimeProjectPort) projectLaunchOutcome(record ports.CommandRecord, out *httpcontract.CommandResponse) error {
	if len(record.Result) != 0 {
		return json.Unmarshal(record.Result, out)
	}
	p.manual.mu.Lock()
	defer p.manual.mu.Unlock()
	if run := p.manual.projectRun; out.Action == "project_launch_start" && run != nil && run.ID == out.RequestID {
		out.Status = "applied"
	} else if out.Action == "project_launch_pr_start" && p.manual.prRunByID(out.RequestID) != nil {
		out.Status = "applied"
	} else {
		out.Diagnostic = "原项目启动操作结果待核实"
	}
	return nil
}
