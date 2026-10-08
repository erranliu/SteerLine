package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

const viewFileVersion = 2
const maxViewFileBytes = 4 << 20

var savedRequestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

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
	ConfigPath  string                    `json:"config_path"`
	RunDir      string                    `json:"run_dir"`
	Drafts      map[string]CandidateDraft `json:"candidate_drafts,omitempty"`
	Outstanding map[string]string         `json:"outstanding,omitempty"`
	Actions     map[string]SavedAction    `json:"actions,omitempty"`
}

type ViewFile struct {
	Version   int                    `json:"version"`
	UpdatedAt time.Time              `json:"updated_at"`
	Projects  map[string]ProjectView `json:"projects"`
}

func emptyViewFile() ViewFile {
	return ViewFile{Version: viewFileVersion, UpdatedAt: time.Now().UTC(), Projects: map[string]ProjectView{}}
}

// viewProfile holds an OS lock for one console window. A second live window
// gets its own directory; once that window exits, a later launch reuses its
// most recently used operation journal and restores only drafts and requests.
type viewProfile struct {
	root string
	dir  string
	lock *viewLock
}

func acquireViewProfile(home string) (*viewProfile, ViewFile, string, error) {
	if strings.TrimSpace(home) == "" {
		return nil, ViewFile{}, "", errors.New("application home directory is required")
	}
	resolvedHome, err := resolveConsolePath(home)
	if err != nil {
		return nil, ViewFile{}, "", fmt.Errorf("resolve application home: %w", err)
	}
	root := filepath.Join(resolvedHome, "ui", "views")
	resolvedRoot, err := resolveConsolePath(root)
	if err != nil {
		return nil, ViewFile{}, "", fmt.Errorf("resolve UI views directory: %w", err)
	}
	if !consolePathWithin(resolvedHome, resolvedRoot) {
		return nil, ViewFile{}, "", errors.New("UI views directory resolves outside application home")
	}
	root = resolvedRoot
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, ViewFile{}, "", err
	}
	resolvedRoot, err = resolveConsolePath(root)
	if err != nil {
		return nil, ViewFile{}, "", fmt.Errorf("resolve created UI views directory: %w", err)
	}
	if !consolePathWithin(resolvedHome, resolvedRoot) {
		return nil, ViewFile{}, "", errors.New("UI views directory changed to a path outside application home")
	}
	root = resolvedRoot
	entries, err := os.ReadDir(root)
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
		resolvedDir, resolveErr := resolveConsolePath(dir)
		if resolveErr != nil {
			return nil, ViewFile{}, "", fmt.Errorf("resolve UI profile directory: %w", resolveErr)
		}
		if !consolePathWithin(root, resolvedDir) {
			return nil, ViewFile{}, "", errors.New("UI profile directory resolves outside its managed root")
		}
		dir = resolvedDir
		viewPath := filepath.Join(dir, "view.json")
		if !consolePathWithin(root, viewPath) {
			return nil, ViewFile{}, "", errors.New("UI journal resolves outside its managed root")
		}
		lock, lockErr := acquireViewLock(root, dir)
		if lockErr != nil {
			continue
		}
		view, readErr := readViewFile(viewPath)
		if _, legacyErr := os.Lstat(filepath.Join(dir, "view.v1.json")); legacyErr == nil {
			readErr = errors.New("retired UI journal exists; preserve it for manual reconciliation")
			view = ViewFile{}
		}
		stamp := time.Time{}
		diagnostic := ""
		if readErr == nil {
			stamp = view.UpdatedAt
		} else {
			diagnostic = "操作记录损坏或版本不受支持；保留原文件，仅允许只读查看与原请求人工对账"
			if info, statErr := os.Stat(filepath.Join(dir, "view.json")); statErr == nil {
				stamp = info.ModTime()
			} else if info, legacyStatErr := os.Stat(filepath.Join(dir, "view.v1.json")); legacyStatErr == nil {
				stamp = info.ModTime()
			}
		}
		available = append(available, candidate{profile: &viewProfile{root: root, dir: dir, lock: lock}, view: view, stamp: stamp, diagnostic: diagnostic})
	}
	if len(available) == 0 {
		id := newRequestID()
		dir := filepath.Join(root, id)
		if !consolePathWithin(root, dir) {
			return nil, ViewFile{}, "", errors.New("new UI profile path escapes its managed root")
		}
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, ViewFile{}, "", err
		}
		resolvedDir, resolveErr := resolveConsolePath(dir)
		if resolveErr != nil || !consolePathWithin(root, resolvedDir) {
			return nil, ViewFile{}, "", errors.New("view profile directory resolves outside its managed root")
		}
		dir = resolvedDir
		lock, lockErr := acquireViewLock(root, dir)
		if lockErr != nil {
			return nil, ViewFile{}, "", lockErr
		}
		return &viewProfile{root: root, dir: dir, lock: lock}, emptyViewFile(), "", nil
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
			diagnostic = "操作记录版本不受支持；保留原文件，禁止新外发"
		}
	}
	return selected.profile, selected.view, diagnostic, nil
}

func acquireViewLock(root, dir string) (*viewLock, error) {
	lockPath := filepath.Join(dir, "steerline.lock")
	if !consolePathWithin(root, dir) || !consolePathWithin(root, lockPath) {
		return nil, errors.New("UI lock resolves outside its managed root")
	}
	return lockView(dir)
}

func insideViewRoot(root, target string) bool { return consolePathWithin(root, target) }

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
	return filepath.Join(p.dir, "view.json")
}

func (p *viewProfile) write(view ViewFile) error {
	path := p.path()
	root := p.root
	if root == "" {
		root = p.dir // Legacy in-package fixtures construct a profile without the production root.
	}
	if !consolePathWithin(root, p.dir) || !consolePathWithin(root, path) {
		return errors.New("UI journal resolves outside its managed root")
	}
	return writeViewFile(path, view)
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
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&view); err != nil {
		return ViewFile{}, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ViewFile{}, errors.New("operation journal contains trailing data")
	}
	if view.Version != viewFileVersion || view.Projects == nil {
		return ViewFile{}, errors.New("view preferences have an unsupported format")
	}
	for key, project := range view.Projects {
		if key == "" || len(view.Projects) > 1024 || len(project.Outstanding) > 256 || len(project.Actions) > 256 || len(project.Drafts) > 4096 {
			return ViewFile{}, errors.New("operation journal contains invalid or excessive state")
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
		for id, draft := range project.Drafts {
			if id == "" || draft.OptionIndex < -1 {
				return ViewFile{}, errors.New("view preferences contain invalid candidate draft state")
			}
		}
	}
	return view, nil
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
