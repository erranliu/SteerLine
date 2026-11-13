package app

import (
	"context"
	"encoding/json"
	"errors"
	roadmapmodel "github.com/erranliu/SteerLine/internal/roadmap/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/roadmap"
)

type roadmapPreparation struct {
	Commit          string                   `json:"commit"`
	PublishedCommit string                   `json:"published_commit"`
	Ready           bool                     `json:"ready"`
	LocalReady      bool                     `json:"local_ready"`
	Branch          string                   `json:"branch"`
	Files           []roadmapPreparationFile `json:"files"`
	Diagnostic      string                   `json:"diagnostic,omitempty"`
}
type roadmapPreparationFile struct {
	Path       string `json:"path"`
	Ready      bool   `json:"ready"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

// A generated local file does not grant readiness until committed. Listing
// observes fixed Git bytes without preparing files, fetching or calling a model.
func (p *roadmapPort) preparation(ctx context.Context) roadmapPreparation {
	p.preparationMu.Lock()
	defer p.preparationMu.Unlock()
	ctx = materialReads(ctx)
	state := roadmapPreparation{Branch: p.config.Branch, Files: []roadmapPreparationFile{}}
	m := p.config.runtimeMaintenance
	if m == nil {
		state.Diagnostic = "项目 Git 不可用"
		return state
	}
	head, err := m.git(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		state.Diagnostic = err.Error()
		return state
	}
	state.Commit, state.Ready = strings.TrimSpace(string(head)), true
	published, publishedErr := m.git(ctx, "rev-parse", "--verify", "refs/remotes/"+p.config.Remote+"/"+p.config.Branch+"^{commit}")
	if publishedErr == nil {
		state.PublishedCommit = strings.TrimSpace(string(published))
	}
	_, err = roadmap.PrepareStatic(p.config.Repository, true)
	state.LocalReady = err == nil
	if cached := p.preparationCache; cached != nil && publishedErr == nil && cached.Commit == state.Commit && cached.PublishedCommit == state.PublishedCommit {
		state.Ready = cached.Ready
		state.Files = append([]roadmapPreparationFile(nil), cached.Files...)
		return state
	}
	cacheable := publishedErr == nil
	for _, item := range []struct {
		path string
		load func() ([]byte, error)
	}{
		{roadmap.ParseRulesPath, prompts.RoadmapParseRules},
		{roadmap.DocumentRulesPath, prompts.RoadmapDocumentRules},
		{"SteerlineFlow/spec/SpecGuide.md", nil},
	} {
		file := roadmapPreparationFile{Path: item.path}
		body, err := p.materials.readCommittedMaterial(ctx, state.Commit, item.path)
		if err == nil {
			if publishedErr != nil {
				err = errors.New("尚未核实目标分支，请在项目维护中检查更新")
			} else {
				fixed, readErr := p.materials.readCommittedMaterial(ctx, state.PublishedCommit, item.path)
				if readErr != nil {
					err = readErr
				} else if !prompts.SameRules(body, fixed) {
					err = errors.New("当前静态材料尚未发布到目标分支，或本地尚未更新")
				}
			}
		}
		if err != nil {
			var missing *missingCommittedFileError
			if !errors.As(err, &missing) {
				cacheable = false
			}
		}
		if err == nil && item.load != nil {
			expected, loadErr := item.load()
			if loadErr != nil {
				err = loadErr
			} else if !prompts.SameRules(body, expected) {
				err = errors.New("静态规则与当前版本不同，需审阅更新")
			}
		}
		if err == nil && (len(body) > roadmapmodel.MaxBytes || !utf8.Valid(body) || len(strings.TrimSpace(string(body))) == 0) {
			err = errors.New("静态材料为空、过大或不是 UTF-8")
		}
		file.Ready = err == nil
		if err != nil {
			state.Ready = false
			file.Diagnostic = err.Error()
		}
		state.Files = append(state.Files, file)
	}
	if cacheable {
		copy := state
		copy.Files = append([]roadmapPreparationFile(nil), state.Files...)
		p.preparationCache = &copy
	}
	return state
}

// The init intent only ensures non-overwriting static files. After interruption,
// reconcile that property without rerunning filesystem writes or claiming who
// created a file. A partial result is explicitly rejected with files retained.
func (p *runtimeProjectPort) reconcilePreparation(ctx context.Context, command storageports.CommandRecord, envelope httpcontract.CommandEnvelope) (*httpcontract.CommandResponse, error) {
	port := p.config.runtimeRoadmaps
	port.publicationMu.Lock()
	defer port.publicationMu.Unlock()
	out := maintenanceResponse(envelope)
	out.Status = "rejected"
	out.Code = "initialization_incomplete"
	out.Diagnostic = "原初始化未能核实完整完成，现有文件保留；请刷新核对后显式重新初始化"
	var input struct {
		Commit string `json:"commit"`
	}
	if decodeStrictJSON(envelope.Payload, &input) == nil && envelope.TargetID == httpcontract.ProjectID(p.config.Identity()) && envelope.ExpectedVersion == 1 && port.config.runtimeMaintenance != nil {
		workspace, err := port.config.runtimeMaintenance.workspace(ctx)
		if err != nil {
			return nil, err
		}
		if strings.Split(workspace, "\n")[0] == input.Commit {
			if _, err = roadmap.PrepareStatic(port.config.Repository, true); err == nil {
				out.Status = "applied"
				out.Code = ""
				out.Diagnostic = ""
				out.Roadmap, _ = json.Marshal(struct {
					Reconciled  bool               `json:"reconciled"`
					Preparation roadmapPreparation `json:"preparation"`
				}{true, port.preparation(ctx)})
			} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, roadmap.ErrStaticRuleConflict) {
				return nil, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command.Status = out.Status
	command.Result, _ = json.Marshal(out)
	command.UpdatedAt = time.Now().UTC()
	if err := p.store.UpdateCommand(context.WithoutCancel(ctx), command); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *roadmapPort) prepareStatic(ctx context.Context, in httpcontract.CommandEnvelope) (any, error) {
	var input struct {
		Commit string `json:"commit"`
	}
	if err := decodeStrictJSON(in.Payload, &input); err != nil {
		return nil, err
	}
	if in.TargetID != httpcontract.ProjectID(p.config.Identity()) || in.ExpectedVersion != 1 {
		return nil, errors.New("初始化目标不是当前项目")
	}
	p.publicationMu.Lock()
	defer p.publicationMu.Unlock()
	m := p.config.runtimeMaintenance
	if m == nil {
		return nil, errors.New("项目 Git 不可用")
	}
	before, err := m.workspace(ctx)
	if err != nil {
		return nil, err
	}
	if input.Commit == "" || strings.Split(before, "\n")[0] != input.Commit {
		return nil, errors.New("项目版本已变化，请刷新后初始化")
	}
	files, err := roadmap.PrepareStatic(p.config.Repository, false)
	if err != nil {
		return nil, err
	}
	after, err := m.workspace(ctx)
	if err != nil || before != after {
		return nil, errors.New("初始化期间 Git 状态变化，文件可能已生成，请刷新核对")
	}
	return struct {
		Files       []roadmap.PreparedFile `json:"files"`
		Preparation roadmapPreparation     `json:"preparation"`
	}{files, p.preparation(ctx)}, nil
}
