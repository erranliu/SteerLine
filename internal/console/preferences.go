package console

import (
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const viewFileVersion = 1
const maxViewFileBytes = 4 << 20

var savedRequestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type FoldChoice struct {
	Manual        *bool `json:"manual,omitempty"`
	KeepExpanded  bool  `json:"keep_expanded,omitempty"`
	AutoProcessed bool  `json:"auto_processed,omitempty"`
}

type SavedAction struct {
	DecisionIdentity       string                `json:"decision_identity,omitempty"`
	Payload                api.ControlPayload    `json:"payload,omitempty"`
	EnvironmentTarget      *api.HostActionTarget `json:"environment_target,omitempty"`
	DecisionDraftOptionID  string                `json:"draft_option,omitempty"`
	DecisionDraftNote      string                `json:"draft_note,omitempty"`
	DecisionRevision       uint64                `json:"decision_revision,omitempty"`
	DecisionVersion        uint64                `json:"decision_version,omitempty"`
	Action                 string                `json:"action,omitempty"`
	TaskID                 string                `json:"task_id,omitempty"`
	ExecutionProfileID     string                `json:"execution_profile_id,omitempty"`
	ExecutionProfileDigest string                `json:"execution_profile_digest,omitempty"`
	DecisionID             string                `json:"decision_id,omitempty"`
	Status                 string                `json:"status"`
	RunID                  string                `json:"run_id,omitempty"`
	TargetInstance         string                `json:"target_instance,omitempty"`
	UpdatedAt              time.Time             `json:"updated_at,omitempty"`
}

type ProjectView struct {
	Candidates       map[string]string           `json:"candidates,omitempty"`
	Reading          map[string]CandidateReading `json:"candidate_reading,omitempty"`
	Drafts           map[string]CandidateDraft   `json:"candidate_drafts,omitempty"`
	ConfigPath       string                      `json:"config_path"`
	RunDir           string                      `json:"run_dir"`
	SelectedKey      string                      `json:"selected_key,omitempty"`
	SelectionPending bool                        `json:"selection_pending,omitempty"`
	Filter           string                      `json:"filter,omitempty"`
	Panel            string                      `json:"panel,omitempty"`
	AttemptID        string                      `json:"attempt_id,omitempty"`
	AttemptPinned    bool                        `json:"attempt_pinned,omitempty"` // Only an explicit history choice pins an attempt across refreshes and restarts.
	Follow           bool                        `json:"follow"`
	AnchorGroup      string                      `json:"anchor_group,omitempty"`
	AnchorItem       string                      `json:"anchor_item,omitempty"`
	AnchorRevision   int64                       `json:"anchor_revision,omitempty"`
	AnchorLine       int                         `json:"anchor_line,omitempty"`
	ReadGroup        string                      `json:"read_group,omitempty"`
	ReadItem         string                      `json:"read_item,omitempty"`
	ReadRevision     int64                       `json:"read_revision,omitempty"`
	ReadLine         int                         `json:"read_line,omitempty"`
	BodyOffset       int64                       `json:"body_offset,omitempty"`
	BodyPages        []int64                     `json:"body_pages,omitempty"`
	BodyKind         string                      `json:"body_kind,omitempty"`
	DetailScroll     int                         `json:"detail_scroll,omitempty"`
	LogScroll        int                         `json:"log_scroll,omitempty"`
	Outstanding      map[string]string           `json:"outstanding,omitempty"`
	Actions          map[string]SavedAction      `json:"actions,omitempty"`
	Groups           map[string]FoldChoice       `json:"groups,omitempty"`
}

type ViewFile struct {
	Version       int                    `json:"version"`
	UpdatedAt     time.Time              `json:"updated_at"`
	ActiveProject string                 `json:"active_project,omitempty"`
	Projects      map[string]ProjectView `json:"projects"`
}

func emptyViewFile() ViewFile {
	return ViewFile{Version: viewFileVersion, UpdatedAt: time.Now().UTC(), Projects: map[string]ProjectView{}}
}

// viewProfile holds an OS lock for one console window. A second live window
// gets its own directory; once that window exits, a later launch reuses its
// most recently used profile and restores its own view choices.
type viewProfile struct {
	dir  string
	lock *viewLock
}

func acquireViewProfile(home string) (*viewProfile, ViewFile, string, error) {
	if strings.TrimSpace(home) == "" {
		return nil, ViewFile{}, "", errors.New("application home directory is required")
	}
	root := filepath.Join(home, "ui", "views")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, ViewFile{}, "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, ViewFile{}, "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, ViewFile{}, "", err
	}
	type candidate struct {
		profile    *viewProfile
		view       ViewFile
		stamp      time.Time
		diagnostic string
	}
	var available []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		resolvedDir, resolveErr := filepath.EvalSymlinks(dir)
		if resolveErr != nil || !insideViewRoot(resolvedRoot, resolvedDir) {
			continue
		}
		lock, lockErr := lockView(dir)
		if lockErr != nil {
			continue
		}
		view, readErr := readViewFile(filepath.Join(dir, "view.v1.json"))
		stamp := time.Time{}
		diagnostic := ""
		if readErr == nil {
			stamp = view.UpdatedAt
		} else {
			diagnostic = "视图偏好损坏或版本不受支持，已重置"
			if info, statErr := os.Stat(filepath.Join(dir, "view.v1.json")); statErr == nil {
				stamp = info.ModTime()
			}
		}
		available = append(available, candidate{profile: &viewProfile{dir: dir, lock: lock}, view: view, stamp: stamp, diagnostic: diagnostic})
	}
	if len(available) == 0 {
		id := newRequestID()
		dir := filepath.Join(root, id)
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, ViewFile{}, "", err
		}
		resolvedDir, resolveErr := filepath.EvalSymlinks(dir)
		if resolveErr != nil || !insideViewRoot(resolvedRoot, resolvedDir) {
			return nil, ViewFile{}, "", errors.New("view profile directory resolves outside its managed root")
		}
		lock, lockErr := lockView(dir)
		if lockErr != nil {
			return nil, ViewFile{}, "", lockErr
		}
		return &viewProfile{dir: dir, lock: lock}, emptyViewFile(), "", nil
	}
	sort.SliceStable(available, func(i, j int) bool { return available[i].stamp.After(available[j].stamp) })
	selected := available[0]
	for _, candidate := range available[1:] {
		candidate.profile.close()
	}
	var diagnostic string
	if selected.view.Version != viewFileVersion {
		selected.view = emptyViewFile()
		diagnostic = selected.diagnostic
		if diagnostic == "" {
			diagnostic = "视图偏好版本不受支持，已重置"
		}
	}
	return selected.profile, selected.view, diagnostic, nil
}

func insideViewRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (p *viewProfile) close() {
	if p != nil && p.lock != nil {
		p.lock.Close()
		p.lock = nil
	}
}

func (p *viewProfile) path() string {
	if p == nil {
		return ""
	}
	return filepath.Join(p.dir, "view.v1.json")
}

func readViewFile(path string) (ViewFile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyViewFile(), nil
		}
		return ViewFile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ViewFile{}, errors.New("view preferences must not be a symbolic link")
	}
	file, err := os.Open(path)
	if err != nil {
		return ViewFile{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxViewFileBytes+1))
	if err != nil {
		return ViewFile{}, err
	}
	if len(data) == 0 || len(data) > maxViewFileBytes {
		return ViewFile{}, errors.New("view preferences have an invalid size")
	}
	var view ViewFile
	if err = json.Unmarshal(data, &view); err != nil {
		return ViewFile{}, err
	}
	if view.Version != viewFileVersion || view.Projects == nil {
		return ViewFile{}, errors.New("view preferences have an unsupported format")
	}
	for key, project := range view.Projects {
		if key == "" || len(view.Projects) > 1024 || (project.Groups != nil && len(project.Groups) > 50000) || len(project.Outstanding) > 256 || len(project.Actions) > 256 || len(project.Candidates) > 1024 || len(project.Reading) > 4096 || len(project.Drafts) > 4096 || project.AnchorRevision < 0 || project.AnchorLine < 0 || project.ReadRevision < 0 || project.ReadLine < 0 || project.BodyOffset < 0 || project.DetailScroll < 0 || project.LogScroll < 0 || !validBodyPages(project.BodyPages, project.BodyOffset) || !validSavedFilter(project.Filter) || !validSavedPanel(project.Panel) || !validBodyKind(project.BodyKind) {
			return ViewFile{}, errors.New("view preferences contain invalid or excessive state")
		}
		for requestID, taskKey := range project.Outstanding {
			action, exists := project.Actions[requestID]
			if !savedRequestIDPattern.MatchString(requestID) || !exists || taskKey != "task:"+action.TaskID || action.Status == "applied" || action.Status == "rejected" || action.Status == "failed" {
				return ViewFile{}, errors.New("view preferences contain an invalid request identity")
			}
		}
		for requestID, action := range project.Actions {
			if !savedRequestIDPattern.MatchString(requestID) || !validSavedAction(action.Action) || !validSavedActionStatus(action.Status) || action.TargetInstance == "" {
				return ViewFile{}, errors.New("view preferences contain an invalid action record")
			}
		}
		for groupID := range project.Groups {
			if groupID == "" {
				return ViewFile{}, errors.New("view preferences contain an invalid group identity")
			}
		}
		for id, reading := range project.Reading {
			if id == "" || reading.AnchorRevision < 0 || reading.ReadRevision < 0 || reading.AnchorLine < 0 || reading.ReadLine < 0 || reading.DetailScroll < 0 || reading.LogScroll < 0 || !validBodyKind(reading.BodyKind) || !validBodyPages(reading.BodyPages, reading.BodyOffset) {
				return ViewFile{}, errors.New("view preferences contain invalid candidate reading state")
			}
		}
		for id, draft := range project.Drafts {
			if id == "" || draft.OptionIndex < -1 {
				return ViewFile{}, errors.New("view preferences contain invalid candidate draft state")
			}
		}
	}
	return view, nil
}

func validSavedFilter(value string) bool {
	switch value {
	case "", "all", "ready", "running", "decision", "merge", "attention", "history":
		return true
	}
	return false
}
func validSavedPanel(value string) bool {
	switch value {
	case "", "detail", "activity", "session", "final", "list", "decisions", "recovery":
		return true
	}
	return false
}
func validBodyKind(value string) bool { return value == "" || value == "process" || value == "final" }
func validBodyPages(values []int64, current int64) bool {
	if len(values) == 0 {
		return current == 0
	}
	if len(values) > 257 || values[0] != 0 {
		return false
	}
	found := false
	for i, value := range values {
		if value < 0 || i > 0 && value <= values[i-1] {
			return false
		}
		if value == current {
			found = true
		}
	}
	return found
}
func validSavedAction(value string) bool {
	switch value {
	case "open_terminal", "recheck_environment", "start_candidate", "abandon_candidate", "start_task", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service", "answer_decision":
		return true
	}
	return false
}
func validSavedActionStatus(value string) bool {
	switch value {
	case "submitted", "pending", "in_flight", "applying", "applied", "accepted", "rejected", "failed", "indeterminate", "submitting":
		return true
	}
	return false
}

func writeViewFile(path string, view ViewFile) error {
	if view.Version != viewFileVersion {
		return errors.New("refusing to write an unsupported view preference version")
	}
	view.UpdatedAt = time.Now().UTC()
	if view.Projects == nil {
		view.Projects = map[string]ProjectView{}
	}
	data, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxViewFileBytes {
		return errors.New("view preferences exceed size limit")
	}
	return writeViewAtomic(path, append(data, '\n'))
}
