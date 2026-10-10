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
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	app "github.com/erranliu/SteerLine/internal/app"
	host "github.com/erranliu/SteerLine/internal/app/host"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
	"github.com/erranliu/SteerLine/tests/e2e/internal/native"
	"github.com/google/uuid"
)

const debugRepository = "erranliu/SteerlineTest"
const debugRemote = "https://github.com/" + debugRepository + ".git"

type debugRelease struct{ root, home, repo, config, provider, helper, binary, base, status string }

// DebugReleaseMain is the package-local runtime entry point, never a production command.
func DebugReleaseMain(args []string) error {
	if len(args) == 0 {
		return errors.New("debug requires init|run|console|reset|merge|control|snapshot")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	d, err := openDebugRelease(filepath.Dir(filepath.Dir(exe)))
	if err != nil {
		return err
	}
	if args[0] != "init" && d.status != "ready" {
		return errors.New("debug package generation has not completed")
	}
	for _, a := range args[1:] {
		if a != "--simulate" && a != "--check" && a != "--no-pause" && args[0] != "control" {
			return fmt.Errorf("unknown debug argument %q", a)
		}
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
	if args[0] == "run-selected" {
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
		args[0] = "run"
	}
	if err = d.prepare(simulate); err != nil {
		return err
	}
	switch args[0] {
	case "prepare":
		return nil
	case "run":
		return d.run()
	case "console":
		cmd := d.command(d.binary, "console", "-config", d.config)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
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
		Version int    `json:"version"`
		Base    string `json:"base_branch"`
		Status  string `json:"status"`
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
	d := &debugRelease{root: root, home: filepath.Join(root, "home"), repo: filepath.Join(root, "projects", "SteerlineTest"), config: filepath.Join(root, "home", "etc", "SteerlineTest.json"), provider: filepath.Join(root, "provider", "state.json"), helper: filepath.Join(root, "bin", "debug-helper.exe"), binary: filepath.Join(root, "bin", "steerline.exe"), base: manifest.Base, status: manifest.Status}
	if _, err = os.Stat(filepath.Join(d.repo, ".git", "objects", "info", "alternates")); err == nil {
		return nil, errors.New("Git alternates are forbidden")
	}
	// All materialized config paths must remain local; git/gh/codex are explicit machine tools.
	for _, name := range []string{"SteerlineTest.json", "SteerlineTest.live.json", "SteerlineTest.simulate.json"} {
		var cfg map[string]any
		raw, err = os.ReadFile(filepath.Join(d.home, "etc", name))
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
	return Client{Binary: d.binary, Config: d.config, Home: d.home, StatePath: d.provider, LogDir: filepath.Join(d.root, "evidence", "commands")}
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
	for _, args := range [][]string{{"init", "--quiet", "-b", d.base}, {"config", "user.name", "SteerLine Debug"}, {"config", "user.email", "debug@example.invalid"}, {"config", "commit.gpgsign", "false"}, {"config", "core.autocrlf", "false"}, {"config", "core.hooksPath", filepath.Join(d.home, "hooks")}, {"config", "gc.auto", "0"}, {"remote", "add", "origin", debugRemote}, {"add", "."}, {"commit", "--quiet", "-m", "SteerlineTest debug fixture"}} {
		if _, err := d.git(args...); err != nil {
			return err
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
		b, e := os.ReadFile(filepath.Join(d.repo, filepath.FromSlash(p)))
		sum := sha256.Sum256(b)
		return taskmodel.FileReference{Path: p, Commit: head, SHA256: hex.EncodeToString(sum[:])}, e
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
	path := filepath.Join(d.root, "seed", "bound-tasks.json")
	if err = controlled.Write(path, batch); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := d.client()
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
	return controlled.Write(filepath.Join(d.root, "evidence", "initialization.json"), map[string]any{"validation": valid, "receipt": receipt})
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
	// Keep only the selected source profile so the console cannot mix provider schemes.
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
	} else {
		remote := filepath.Join(d.root, "remotes", filepath.FromSlash(debugRepository)+".git")
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
			if state.RepositoryName() != debugRepository || state.BranchName() != d.base {
				return errors.New("provider seed must target this SteerlineTest base")
			}
			if err = controlled.Write(d.provider, state); err != nil {
				return err
			}
		}
	}
	raw, err = os.ReadFile(filepath.Join(d.home, "etc", "SteerlineTest."+mode+".json"))
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

func (d *debugRelease) run() error {
	runtimeDir := filepath.Join(d.root, "runtime")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return err
	}
	lock, err := host.LockRepository(runtimeDir)
	if err != nil {
		return fmt.Errorf("debug service already running: %w", err)
	}
	defer lock.Close()
	var selected struct {
		Scheme string `json:"scheme"`
	}
	raw, err := os.ReadFile(filepath.Join(d.root, "scheme.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &selected); err != nil {
		return err
	}
	fmt.Println("Debug scheme:", selected.Scheme, "Home:", d.home)
	var gitServer *testprocess.Process
	if selected.Scheme == "simulate" {
		addressPath := filepath.Join(d.root, "provider", "address.json")
		os.Remove(addressPath)
		cmd := d.command(d.helper, "git-server", "--root", filepath.Join(d.root, "remotes"), "--address-file", addressPath, "--e2e-state", d.provider)
		log, err := os.OpenFile(filepath.Join(d.root, "logs", "git-server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer log.Close()
		cmd.Stdout, cmd.Stderr = log, log
		gitServer, err = testprocess.Start(cmd)
		if err != nil {
			return err
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			gitServer.Stop(ctx)
		}()
		deadline := time.Now().Add(15 * time.Second)
		var address struct {
			Address string `json:"address"`
		}
		for time.Now().Before(deadline) {
			if raw, e := os.ReadFile(addressPath); e == nil && json.Unmarshal(raw, &address) == nil && address.Address != "" {
				break
			}
			select {
			case <-gitServer.Done:
				return errors.New("Git helper exited; see logs/git-server.log")
			default:
			}
			time.Sleep(50 * time.Millisecond)
		}
		if address.Address == "" {
			return errors.New("Git helper readiness timed out")
		}
		state, err := controlled.Read(d.provider)
		if err != nil {
			return err
		}
		state.GitURL = "http://" + address.Address + "/" + debugRepository + ".git"
		if err = controlled.Write(d.provider, state); err != nil {
			return err
		}
	} else {
		// A fixture base has its own history, so no checkout/fetch of main or external local repo is needed.
		cmd := exec.Command("git", "-C", d.repo, "-c", "credential.helper=", "push", "origin", d.base+":refs/heads/"+d.base)
		cmd.Env = os.Environ()
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
	}
	serviceArgs := []string{"run", "-config", d.config, "-dispatch", "manual", "-console", "summary"}
	if debugVisibleConsole() {
		serviceArgs = append(serviceArgs, "-service-window")
	}
	cmd := d.command(d.binary, serviceArgs...)
	if selected.Scheme == "live" {
		cmd.Env = os.Environ()
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	service, err := testprocess.Start(cmd)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		service.Stop(ctx)
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	select {
	case <-service.Done:
		return service.Wait(context.Background())
	case <-signals:
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		stop := d.command(d.binary, "stop-service", "-config", d.config)
		if selected.Scheme == "live" {
			stop.Env = os.Environ()
		}
		stop.Stdout, stop.Stderr = os.Stdout, os.Stderr
		if err = testprocess.Run(ctx, stop); err != nil {
			return err
		}
		return service.Wait(ctx)
	}
}
