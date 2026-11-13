package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/app/host"
	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type manualPayload struct {
	Head     string    `json:"head"`
	Source   mv.Source `json:"source,omitempty"`
	Attempt  string    `json:"attempt_id,omitempty"`
	Check    string    `json:"check_id,omitempty"`
	Outcome  string    `json:"outcome,omitempty"`
	Notes    string    `json:"notes,omitempty"`
	Evidence []string  `json:"evidence,omitempty"`
}

func (s *manualVerification) records(ctx context.Context) ([]mv.Record, error) {
	rows, err := s.store.ManualVerifications(ctx, s.config.Identity())
	if err != nil {
		return nil, err
	}
	snapshot, err := s.store.ReadRunnerSnapshot(ctx, s.config.Identity())
	if err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, c := range snapshot.Candidates {
		active[c.ID] = c.Participation == "active" && !c.StopRequested && snapshot.Graph.ByID[c.TaskID].Status == "in_progress"
	}
	for i := range rows {
		r := &rows[i]
		r.Archived = r.Archived || !r.Merged && !active[r.Candidate]
		for j := range r.Attempts {
			a := &r.Attempts[j]
			s.mu.Lock()
			running := s.stops[a.ID] != nil
			s.mu.Unlock()
			if a.Active() && !running {
				a.Status = "unknown"
				a.Diagnostic = "原启动尚未完成对账"
			}
			a.Process = nil
		}
	}
	return rows, nil
}

func (s *manualVerification) recover(ctx context.Context) error {
	rows, err := s.store.ManualVerifications(ctx, s.config.Identity())
	if err != nil {
		return err
	}
	for _, r := range rows {
		for _, a := range r.Attempts {
			if !a.Active() {
				continue
			}
			// Before the expanded command is saved no process can have started.
			if verificationTreeAbsent(a) {
				if err := s.patch(r.Candidate, a.ID, func(v *mv.Attempt) { v.Status = "stopped"; v.Diagnostic = "宿主重启后已核对原启动" }); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (p *runtimeProjectPort) manualVerificationCommand(ctx context.Context, in httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error) {
	out := maintenanceResponse(in)
	if err := in.Validate(httpcontract.ProjectID(p.config.Identity()), p.instance); err != nil {
		return out, err
	}
	if p.manual == nil || p.stopping.Load() {
		return out, projectStoppingError()
	}
	s := p.manual
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	var payload manualPayload
	if err := decodeStrictJSON(in.Payload, &payload); err != nil {
		return out, err
	}
	if !taskmodel.ValidCommit(payload.Head) {
		return out, errors.New("核验操作需要完整 Head SHA")
	}
	if in.ExpectedControlRevision != 0 || in.ExpectedGraphRevision != 0 {
		return out, errors.New("人工核验只使用核验记录版本")
	}
	raw, _ := json.Marshal(in)
	reject := func(err error) (httpcontract.CommandResponse, error) {
		out.Status = "rejected"
		out.Code = "manual_verification_rejected"
		out.Diagnostic = err.Error()
		return out, nil
	}
	old, err := s.store.ManualVerificationReceipt(ctx, s.config.Identity(), in.RequestID)
	if err != nil {
		return out, err
	}
	if len(old) > 0 {
		if !bytes.Equal(old, raw) {
			return reject(errors.New("请求 ID 已用于其他操作"))
		}
		out.Status = "applied"
		return out, nil
	}
	r, err := s.store.ManualVerification(ctx, s.config.Identity(), in.TargetID)
	if err != nil {
		return reject(err)
	}
	if r.Revision != in.ExpectedVersion {
		return reject(errors.New("核验记录已更新，请刷新后重试"))
	}
	command := ports.CommandRecord{ProjectIdentity: r.Project, CommandID: in.RequestID, Kind: "manual_verification", TargetID: r.Candidate, ExpectedVersion: in.ExpectedVersion, RequestedBy: host.DefaultRequestedBy(), Payload: raw}
	switch in.Action {
	case "manual_verification_start":
		source := payload.Source
		if source == "" {
			source = mv.SourcePR
		}
		if r.Head != payload.Head || !r.Allows(source, r.Head) {
			return reject(errors.New("核验待办、来源或 PR Head 已变化"))
		}
		target, remote, err := s.refresh(ctx, r)
		if err != nil {
			return reject(err)
		}
		head := r.Head
		if source == mv.SourceRemote {
			head, err = s.remoteVerificationHead(ctx, target, remote)
			if err != nil {
				return reject(err)
			}
		}
		a := mv.Attempt{ID: in.RequestID, Head: head, Source: source, Worktree: s.worktree(in.RequestID), Status: "preparing"}
		created := false
		err = s.store.ApplyManualVerification(ctx, command, func(current *mv.Record) error {
			if current.Head != payload.Head || !current.Allows(a.Source, a.Head) {
				return errors.New("核验待办、来源或 PR Head 已变化")
			}
			for _, previous := range current.Attempts {
				if previous.Source == a.Source && previous.Head == a.Head && previous.Active() {
					return nil
				}
			}
			current.Attempts = append(current.Attempts, a)
			created = true
			return nil
		})
		if err != nil {
			return reject(err)
		}
		if created {
			s.launch(r, a, target, remote)
		}
	case "manual_verification_result":
		i, err := r.Attempt(payload.Attempt)
		if err != nil {
			return reject(err)
		}
		a := r.Attempts[i]
		if a.Head != payload.Head || !r.Allows(a.Source, a.Head) || !a.Started || a.Removed || a.Status == "failed" || a.Status == "unknown" {
			return reject(errors.New("选择该提交上已成功启动的现场"))
		}
		result := mv.Result{Head: a.Head, Source: a.Source, AttemptID: a.ID, CheckID: payload.Check, Outcome: payload.Outcome, Notes: payload.Notes, Evidence: payload.Evidence, Actor: command.RequestedBy, At: time.Now().UTC(), SHA256: r.Definition.SHA256}
		if err := r.ValidateResult(result); err != nil {
			return reject(err)
		}
		if _, _, err := s.refresh(ctx, r); err != nil {
			return reject(err)
		}
		if err := s.verifyWorktree(ctx, a, true); err != nil {
			return reject(err)
		}
		if err := s.store.ApplyManualVerification(ctx, command, func(current *mv.Record) error {
			if !current.Allows(a.Source, a.Head) || current.Attempts[i].Removed {
				return errors.New("原核验现场已变化")
			}
			current.Results = append(current.Results, result)
			return nil
		}); err != nil {
			return reject(err)
		}
	case "manual_verification_close":
		closure := mv.Closure{Source: payload.Source, Head: payload.Head, Actor: command.RequestedBy, At: time.Now().UTC()}
		if _, _, err := s.refresh(ctx, r); err != nil {
			return reject(err)
		}
		if err := s.store.ApplyManualVerification(ctx, command, func(current *mv.Record) error {
			return current.Close(closure)
		}); err != nil {
			return reject(err)
		}
	case "manual_verification_reopen":
		if r.Archived || r.Closure == nil || r.Closure.Head != payload.Head {
			return reject(errors.New("已关闭的核验待办已变化"))
		}
		if err := s.store.ApplyManualVerification(ctx, command, func(current *mv.Record) error {
			if current.Archived {
				return errors.New("核验待办已归档")
			}
			current.Closure = nil
			return nil
		}); err != nil {
			return reject(err)
		}
	case "manual_verification_stop":
		i, err := r.Attempt(payload.Attempt)
		if err != nil {
			return reject(err)
		}
		a := r.Attempts[i]
		if a.Head != payload.Head {
			return reject(errors.New("核验启动记录与提交不匹配"))
		}
		s.mu.Lock()
		stop := s.stops[a.ID]
		s.mu.Unlock()
		if a.Active() && stop == nil {
			return reject(errors.New("原进程身份尚未核实"))
		}
		if err := s.store.ApplyManualVerification(ctx, command, func(*mv.Record) error { return nil }); err != nil {
			return reject(err)
		}
		if stop != nil {
			stop()
		}
	case "manual_verification_remove":
		i, err := r.Attempt(payload.Attempt)
		if err != nil {
			return reject(err)
		}
		a := r.Attempts[i]
		if a.Head != payload.Head || a.Active() {
			return reject(errors.New("请先核实原现场已停止"))
		}
		if !a.Removed {
			if _, err := os.Lstat(a.Worktree); err == nil {
				if err := s.verifyWorktree(ctx, a, false); err != nil {
					return reject(err)
				}
				if _, err := s.git(ctx, "worktree", "remove", a.Worktree); err != nil {
					return reject(err)
				}
			} else if !os.IsNotExist(err) {
				return reject(err)
			}
		}
		if err := s.store.ApplyManualVerification(ctx, command, func(current *mv.Record) error { current.Attempts[i].Removed = true; return nil }); err != nil {
			return reject(err)
		}
	default:
		return reject(errors.New("不支持的人工核验操作"))
	}
	out.Status = "applied"
	return out, nil
}

func (p *runtimeProjectPort) manualVerificationRead(ctx context.Context, route string) (any, error) {
	if p.manual == nil {
		return nil, errors.New("人工核验不可用")
	}
	if route == "/manual-verification" {
		return p.manual.records(ctx)
	}
	parts := strings.Split(strings.TrimPrefix(route, "/manual-verification/"), "/")
	if len(parts) != 3 || parts[1] != "logs" {
		return nil, errors.New("未知的人工核验查询")
	}
	r, err := p.manual.store.ManualVerification(ctx, p.config.Identity(), parts[0])
	if err != nil {
		return nil, err
	}
	i, err := r.Attempt(parts[2])
	if err != nil {
		return nil, err
	}
	return p.manual.logs(r.Attempts[i])
}
