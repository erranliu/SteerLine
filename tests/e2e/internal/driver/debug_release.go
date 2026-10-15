package driver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	app "github.com/erranliu/SteerLine/internal/app"
	host "github.com/erranliu/SteerLine/internal/app/host"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
	"github.com/google/uuid"
)

const debugRepository = "erranliu/SteerlineTest"
const debugRemote = "https://github.com/" + debugRepository + ".git"

type debugRelease struct {
	root, home, repo, config, provider, helper, binary, base, status string
	webProjects                                                      int
}

// DebugReleaseMain is the package-local runtime entry point, never a production command.
func DebugReleaseMain(args []string) error {
	if len(args) == 0 {
		return errors.New("debug requires init|prepare|run|stop|reset|merge|control|snapshot")
	}
	switch args[0] {
	case "init", "init-b", "prepare", "run", "run-selected", "stop", "reset", "merge", "control", "snapshot":
	default:
		return fmt.Errorf("unknown debug operation %q", args[0])
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	d, err := openDebugRelease(filepath.Dir(filepath.Dir(exe)))
	if err != nil {
		return err
	}
	if args[0] != "init" && args[0] != "init-b" && d.status != "ready" {
		return errors.New("debug package generation has not completed")
	}
	for _, a := range args[1:] {
		if a != "--simulate" && a != "--check" && a != "--no-pause" && a != "--no-open" && args[0] != "control" {
			return fmt.Errorf("unknown debug argument %q", a)
		}
	}
	if args[0] == "init-b" {
		return d.secondary().initialize()
	}
	if args[0] == "init" {
		return d.initialize()
	}
	if args[0] == "reset" {
		return d.reset(contains(args, "--check"))
	}
	if args[0] == "control" {
		for i, a := range args[1:] {
			if a == "-config" {
				if i+2 >= len(args) || !strings.EqualFold(filepath.Clean(args[i+2]), d.config) {
					return errors.New("control configuration must be this package's SteerlineTest.json")
				}
			} else if strings.HasPrefix(a, "-config=") {
				return errors.New("use the explicit package -config argument")
			}
		}
	}
	simulate := contains(args, "--simulate")
	if args[0] == "run-selected" || args[0] == "stop" {
		var selected struct {
			Scheme string `json:"scheme"`
		}
		raw, e := os.ReadFile(filepath.Join(d.root, "scheme.json"))
		if e != nil {
			return e
		}
		if e = json.Unmarshal(raw, &selected); e != nil {
			return e
		}
		if selected.Scheme != "live" && selected.Scheme != "simulate" {
			return errors.New("invalid selected scheme")
		}
		simulate = selected.Scheme == "simulate"
		if args[0] == "run-selected" {
			args[0] = "run"
		}
	}
	if err = d.prepare(simulate); err != nil {
		return err
	}
	switch args[0] {
	case "prepare":
		return nil
	case "run":
		return d.run()
	case "stop":
		return d.stop()
	case "merge":
		if !simulate {
			return errors.New("debug merge requires --simulate; merge live PRs explicitly")
		}
		return controlled.Merge(context.Background(), d.provider, d.repo)
	case "control":
		var controlArgs []string
		for _, a := range args[1:] {
			if a != "--simulate" {
				controlArgs = append(controlArgs, a)
			}
		}
		return MainControl(controlArgs)
	case "snapshot":
		common, err := d.git("rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			return err
		}
		snap, err := ReadApplicationProjectSnapshot(context.Background(), filepath.Join(d.stateDir(), "steerline.db"), strings.ToLower(filepath.Clean(common)), "", 100)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(snap)
	}
	return errors.New("unknown debug operation")
}
func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func openDebugRelease(root string) (*debugRelease, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(root, resolved) {
		return nil, errors.New("debug root must not be linked")
	}
	installed := filepath.Clean(`C:\SteerlineRuntime`)
	if pathWithin(installed, root) || pathWithin(root, installed) {
		return nil, errors.New("installed runtime cannot be used for debug")
	}
	if err = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			} // SQLite/journal scratch files may disappear during a read-only scan.
			return err
		}
		if e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("linked debug path: %s", path)
		}
		if !e.IsDir() {
			err := debugPlainFile(path)
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	var manifest struct {
		WebProjects int    `json:"web_projects"`
		Version     int    `json:"version"`
		Base        string `json:"base_branch"`
		Status      string `json:"status"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	if manifest.Version != 1 || !strings.HasPrefix(manifest.Base, "codex/debug/") || (manifest.Status != "building" && manifest.Status != "ready") {
		return nil, errors.New("not a usable generated debug release")
	}
	d := &debugRelease{root: root, home: filepath.Join(root, "home"), repo: filepath.Join(root, "projects", "SteerlineTest"), config: filepath.Join(root, "home", "etc", "SteerlineTest.json"), provider: filepath.Join(root, "provider", "state.json"), helper: filepath.Join(root, "bin", "debug-helper.exe"), binary: filepath.Join(root, "bin", "steerline.exe"), base: manifest.Base, status: manifest.Status, webProjects: manifest.WebProjects}
	if _, err = os.Stat(filepath.Join(d.repo, ".git", "objects", "info", "alternates")); err == nil {
		return nil, errors.New("Git alternates are forbidden")
	}
	// All materialized config paths must remain local; git/gh/codex are explicit machine tools.
	for _, name := range []string{"SteerlineTest.json", "SteerlineTest.live.json", "SteerlineTest.simulate.json"} {
		var cfg map[string]any
		dir := "config-source"
		if name == "SteerlineTest.json" {
			dir = "etc"
		}
		raw, err = os.ReadFile(filepath.Join(d.home, dir, name))
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
		for _, key := range []string{"repository", "run_dir", "worktree_root"} {
			v, _ := cfg[key].(string)
			if key == "run_dir" && v == "" {
				continue
			}
			if !filepath.IsAbs(v) || !pathWithin(root, v) {
				return nil, fmt.Errorf("%s escapes debug package", key)
			}
		}
		if cfg["branch"] != d.base {
			return nil, errors.New("debug base identity changed")
		}
		pr, ok := cfg["pr"].(map[string]any)
		if !ok || pr["repository"] != debugRepository {
			return nil, errors.New("debug PR target must be erranliu/SteerlineTest")
		}
	}
	return d, nil
}
func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func (d *debugRelease) client() Client {
	return Client{Binary: d.binary, Config: d.config, Home: d.home, StatePath: d.provider, LogDir: filepath.Join(d.root, "evidence", "commands", filepath.Base(d.config))}
}
func (d *debugRelease) stateDir() string {
	return filepath.Join(d.home, "state", app.RepositoryStateKey(filepath.Join(d.repo, ".git")))
}
func (d *debugRelease) command(exe string, args ...string) *exec.Cmd {
	cmd := exec.Command(exe, args...)
	cmd.Dir = d.root
	cmd.Env = Environment(os.Environ(), d.home, d.provider)
	var env []string
	for _, entry := range cmd.Env {
		if !strings.HasPrefix(strings.ToUpper(entry), "GH_CONFIG_DIR=") {
			env = append(env, entry)
		}
	}
	cmd.Env = append(env, "GH_CONFIG_DIR="+filepath.Join(d.home, "gh"))
	if exe == d.binary && len(args) > 0 && args[0] == "serve" {
		if devOrigin := os.Getenv("STEERLINE_WEB_DEV_ORIGIN"); devOrigin != "" {
			cmd.Env = append(cmd.Env, "STEERLINE_WEB_DEV_ORIGIN="+devOrigin)
		}
	}
	if token, err := os.ReadFile(filepath.Join(d.home, "github-token")); err == nil {
		cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http."+debugRemote+".extraheader", "GIT_CONFIG_VALUE_0="+debugAuthHeader(token))
	}
	return cmd
}
func debugAuthHeader(token []byte) string {
	return "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+strings.TrimSpace(string(token))))
}
func (d *debugRelease) git(args ...string) (string, error) {
	cmd := d.command("git", append([]string{"-C", d.repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func (d *debugRelease) initialize() error {
	if _, err := os.Stat(filepath.Join(d.repo, ".git")); os.IsNotExist(err) {
		for _, args := range [][]string{{"init", "--quiet", "-b", d.base}, {"config", "user.name", "SteerLine Debug"}, {"config", "user.email", "debug@example.invalid"}, {"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"}, {"config", "core.hooksPath", filepath.Join(d.home, "hooks")}, {"config", "gc.auto", "0"}, {"remote", "add", "origin", debugRemote}, {"add", "."}, {"commit", "--quiet", "-m", "SteerlineTest debug fixture"}} {
			if _, err := d.git(args...); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(d.root, "evidence", "commands"), 0700); err != nil {
		return err
	}
	// Preserve the generated copy of the catalog. Run-time preparation only selects its default.
	raw, err := os.ReadFile(filepath.Join(d.home, "execution.json"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(d.home, "execution.source.json"), raw, 0600); err != nil {
		return err
	}
	var seed struct {
		Tasks []struct {
			Key   string `json:"key"`
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"tasks"`
		Dependencies []struct {
			Predecessor string `json:"predecessor"`
			Successor   string `json:"successor"`
		} `json:"dependencies"`
	}
	raw, err = os.ReadFile(filepath.Join(d.root, "seed", "tasks.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &seed); err != nil {
		return err
	}
	head, err := d.git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	common, err := d.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	reference := func(name string) (taskmodel.FileReference, error) {
		p := "docs/spec/" + name
		// Pin committed bytes so Windows checkout line-ending conversion cannot change the contract hash.
		cmd := d.command("git", "-C", d.repo, "show", head+":"+p)
		b, e := cmd.Output()
		if e != nil {
			return taskmodel.FileReference{}, fmt.Errorf("read committed contract %s: %w", p, e)
		}
		sum := sha256.Sum256(b)
		return taskmodel.FileReference{Path: p, Commit: head, SHA256: hex.EncodeToString(sum[:])}, nil
	}
	spec, err := reference("task.spec.md")
	if err != nil {
		return err
	}
	plan, err := reference("task.plan.md")
	if err != nil {
		return err
	}
	batch := taskmodel.TaskBatchInput{SchemaVersion: taskmodel.BatchSchemaVersion, ProjectID: strings.ToLower(filepath.Clean(filepath.FromSlash(common)))}
	for _, t := range seed.Tasks {
		contract := taskmodel.TaskContract{ContractID: "debug-" + t.Key, Repository: debugRemote, Remote: "origin", Branch: d.base, SpecRef: spec, PlanRef: plan, CommonRefs: []taskmodel.FileReference{}, AcceptanceRefs: []taskmodel.FileReference{spec}}
		contract.Digest, err = taskmodel.ContractDigest(contract)
		if err != nil {
			return err
		}
		batch.Tasks = append(batch.Tasks, taskmodel.TaskBatchTask{LocalKey: t.Key, TaskID: t.ID, Title: t.Title, Type: "task", Contract: contract})
	}
	for _, edge := range seed.Dependencies {
		batch.Dependencies = append(batch.Dependencies, taskmodel.TaskBatchEdge{Predecessor: taskmodel.TaskBatchReference{LocalKey: edge.Predecessor}, Successor: taskmodel.TaskBatchReference{LocalKey: edge.Successor}})
	}
	path := filepath.Join(d.root, "seed", filepath.Base(d.config)+".bound-tasks.json")
	if err = controlled.Write(path, batch); err != nil {
		return err
	}
	return nil
}
func (d *debugRelease) seedTasks() error {
	path := filepath.Join(d.root, "seed", filepath.Base(d.config)+".bound-tasks.json")
	if _, err := os.Stat(filepath.Join(d.root, "evidence", filepath.Base(d.config)+".initialization.json")); err == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var raw []byte
	var err error
	c := d.client()
	if err := os.MkdirAll(c.LogDir, 0700); err != nil {
		return err
	}
	raw, err = c.Run(ctx, "task", "validate", "-config", d.config, "-file", path)
	if err != nil {
		return err
	}
	var valid taskmodel.TaskBatchValidation
	if err = json.Unmarshal(raw, &valid); err != nil {
		return err
	}
	if !valid.Valid {
		return fmt.Errorf("invalid debug seed: %s", raw)
	}
	raw, err = c.Run(ctx, "task", "create", "-config", d.config, "-file", path, "-request-id", strings.ReplaceAll(uuid.NewString(), "-", ""), "-expected-authority-version", strconv.FormatUint(valid.AuthorityVersion, 10), "-by", "debug-generator")
	if err != nil {
		return err
	}
	var receipt taskmodel.TaskBatchReceipt
	if err = json.Unmarshal(raw, &receipt); err != nil {
		return err
	}
	if receipt.Status != "accepted" {
		return fmt.Errorf("seed rejected: %s", raw)
	}
	return controlled.Write(filepath.Join(d.root, "evidence", filepath.Base(d.config)+".initialization.json"), map[string]any{"validation": valid, "receipt": receipt})
}

func (d *debugRelease) prepare(simulate bool) error {
	lock, err := host.LockRepository(d.root)
	if err != nil {
		return fmt.Errorf("debug preparation/reset busy: %w", err)
	}
	defer lock.Close()
	remote, err := d.git("remote", "get-url", "origin")
	if err != nil {
		return err
	}
	if remote != debugRemote {
		return errors.New("debug origin must be the SteerlineTest identity")
	}
	mode := "live"
	if simulate {
		mode = "simulate"
	}
	marker := filepath.Join(d.root, "scheme.json")
	var selected struct {
		Scheme string `json:"scheme"`
		Ready  bool   `json:"ready"`
	}
	if raw, e := os.ReadFile(marker); e == nil {
		if e = json.Unmarshal(raw, &selected); e != nil {
			return e
		}
		if selected.Scheme != mode {
			return fmt.Errorf("package is bound to %s; generate a new package to use %s", selected.Scheme, mode)
		}
		if selected.Ready {
			return d.setEnvironment(mode)
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	// Freeze before any provider side effect. A failed preparation can be retried only in the same scheme.
	if err = controlled.Write(marker, map[string]string{"scheme": mode}); err != nil {
		return err
	}
	if err = d.prepareSelected(mode); err != nil {
		return err
	}
	if d.webProjects == 2 {
		if err = d.secondary().prepareSelected(mode); err != nil {
			return err
		}
		if err = d.setEnvironment(mode); err != nil {
			return err
		}
	}
	return controlled.Write(marker, map[string]any{"scheme": mode, "ready": true})
}
func (d *debugRelease) prepareSelected(mode string) error {
	var catalog map[string]any
	raw, err := os.ReadFile(filepath.Join(d.home, "execution.source.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		return err
	}
	catalog["default_execution_profile"] = mode
	// Keep only the selected source profile so projects cannot mix provider schemes.
	profiles, _ := catalog["execution_profiles"].([]any)
	var selected []any
	for _, p := range profiles {
		m, ok := p.(map[string]any)
		if ok && m["id"] == mode {
			selected = append(selected, p)
		}
	}
	if len(selected) != 1 {
		return errors.New("source execution catalog requires exactly one selected profile")
	}
	catalog["execution_profiles"] = selected
	if mode == "live" {
		if selected[0].(map[string]any)["host"] == "local" {
			authSource := os.Getenv("CODEX_HOME")
			if authSource == "" {
				user, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				authSource = filepath.Join(user, ".codex")
			}
			if pathWithin(`C:\SteerlineRuntime`, authSource) {
				return errors.New("cannot provision debug authentication from installed runtime")
			}
			if resolved, e := filepath.EvalSymlinks(authSource); e == nil && pathWithin(`C:\SteerlineRuntime`, resolved) {
				return errors.New("cannot provision authentication through an installed-runtime alias")
			}
			if _, err = os.Stat(filepath.Join(d.home, "codex", "auth.json")); os.IsNotExist(err) {
				if err = native.SeedAuth(authSource, filepath.Join(d.home, "codex"), "unelevated"); err != nil {
					return err
				}
				// Model/effort come only from the copied execution catalog, not a second config default.
				if err = os.WriteFile(filepath.Join(d.home, "codex", "config.toml"), nil, 0600); err != nil {
					return err
				}
				if err = native.TrustProject(filepath.Join(d.home, "codex"), d.repo); err != nil {
					return err
				}
			}
			codex, err := native.CodexExecutable("codex")
			if err != nil {
				return err
			}
			configs := selected[0].(map[string]any)["implementation_configs"].([]any)
			configs[0].(map[string]any)["payload"].(map[string]any)["executable"] = codex
		}
		// Capture authentication privately, never in command/evidence output.
		if _, err = os.Stat(filepath.Join(d.home, "github-token")); os.IsNotExist(err) {
			cmd := exec.Command("gh", "auth", "token", "--hostname", "github.com")
			token, e := cmd.Output()
			if e != nil || len(strings.TrimSpace(string(token))) == 0 {
				return errors.New("live requires gh authentication")
			}
			if e = os.WriteFile(filepath.Join(d.home, "github-token"), []byte(strings.TrimSpace(string(token))), 0600); e != nil {
				return e
			}
		}
		// The product deliberately strips center GH_TOKEN/Git header variables
		// before starting workers. Provision the package's own gh login instead;
		// never change the user's config or credential-manager/keychain entries.
		if _, err = os.Stat(filepath.Join(d.home, "gh", "hosts.yml")); os.IsNotExist(err) {
			token, e := os.ReadFile(filepath.Join(d.home, "github-token"))
			if e != nil {
				return e
			}
			cmd := exec.Command("gh", "auth", "login", "--hostname", "github.com", "--git-protocol", "https", "--with-token", "--insecure-storage")
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				if !strings.EqualFold(key, "GH_TOKEN") && !strings.EqualFold(key, "GITHUB_TOKEN") && !strings.EqualFold(key, "GH_CONFIG_DIR") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "GH_CONFIG_DIR="+filepath.Join(d.home, "gh"))
			cmd.Stdin = strings.NewReader(strings.TrimSpace(string(token)) + "\n")
			if _, e := cmd.CombinedOutput(); e != nil {
				return errors.New("cannot provision package-owned GitHub login")
			}
		}
	} else {
		remote := filepath.Join(d.remoteRoot(), filepath.FromSlash(debugRepository)+".git")
		if _, err = os.Stat(remote); os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Dir(remote), 0700); err != nil {
				return err
			}
			if _, err = d.git("init", "--bare", "--quiet", remote); err != nil {
				return err
			}
			if _, err = d.git("--git-dir", remote, "config", "http.receivepack", "true"); err != nil {
				return err
			}
			if _, err = d.git("push", "--quiet", remote, d.base); err != nil {
				return err
			}
		}
		if _, err = os.Stat(d.provider); os.IsNotExist(err) {
			raw, err := os.ReadFile(filepath.Join(d.root, "seed", "provider.json"))
			if err != nil {
				return err
			}
			var state controlled.State
			if err = json.Unmarshal(raw, &state); err != nil {
				return err
			}
			if d.isSecondary() {
				state.BaseBranch = d.base
			}
			if state.RepositoryName() != debugRepository || state.BranchName() != d.base {
				return errors.New("provider seed must target this SteerlineTest base")
			}
			if err = controlled.Write(d.provider, state); err != nil {
				return err
			}
		}
	}
	raw, err = os.ReadFile(filepath.Join(d.home, "config-source", strings.TrimSuffix(filepath.Base(d.config), ".json")+"."+mode+".json"))
	if err != nil {
		return err
	}
	if err = os.WriteFile(d.config, raw, 0600); err != nil {
		return err
	}
	if err = controlled.Write(filepath.Join(d.home, "execution.json"), catalog); err != nil {
		return err
	}
	return d.setEnvironment(mode)
}
func (d *debugRelease) setEnvironment(mode string) error {
	env := Environment(os.Environ(), d.home, d.provider)
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if strings.HasPrefix(strings.ToUpper(key), "STEERLINE_") || strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			os.Unsetenv(key)
		}
	}
	for _, entry := range env {
		p := strings.SplitN(entry, "=", 2)
		if len(p) == 2 {
			os.Setenv(p[0], p[1])
		}
	}
	os.Setenv("GH_CONFIG_DIR", filepath.Join(d.home, "gh"))
	if mode == "live" {
		os.Unsetenv("STEERLINE_E2E_CONTROLLED_STATE")
		token, err := os.ReadFile(filepath.Join(d.home, "github-token"))
		if err != nil {
			return err
		}
		os.Setenv("GH_TOKEN", strings.TrimSpace(string(token)))
		os.Setenv("GIT_CONFIG_COUNT", "1")
		os.Setenv("GIT_CONFIG_KEY_0", "http."+debugRemote+".extraheader")
		os.Setenv("GIT_CONFIG_VALUE_0", debugAuthHeader(token))
	}
	return nil
}

func (d *debugRelease) isSecondary() bool { return strings.HasSuffix(d.repo, "SteerlineTestB") }
func (d *debugRelease) suffix() string {
	if d.isSecondary() {
		return "-b"
	}
	return ""
}
func (d *debugRelease) remoteRoot() string { return filepath.Join(d.root, "remotes"+d.suffix()) }
func (d *debugRelease) secondary() *debugRelease {
	copy := *d
	copy.repo = filepath.Join(d.root, "projects", "SteerlineTestB")
	copy.config = filepath.Join(d.home, "etc", "SteerlineTestB.json")
	copy.provider = filepath.Join(d.root, "provider-b", "state.json")
	copy.base = d.base + "-b"
	copy.webProjects = 0
	return &copy
}
