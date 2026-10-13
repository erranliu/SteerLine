// Package consoleapp composes the terminal HTTP client without backend owners.
package consoleapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/console"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
)

type Paths struct {
	Home, Bin, Executable, SourceCommit string
	SourceDirty                         string
}

func canonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
func Load(ctx context.Context, configPath string, paths Paths) ([]api.ProjectRef, error) {
	var files []string
	if configPath != "" {
		absolute, err := filepath.Abs(configPath)
		if err != nil {
			return nil, err
		}
		files = []string{absolute}
	} else {
		var err error
		files, err = filepath.Glob(filepath.Join(paths.Home, "etc", "*.json"))
		if err != nil {
			return nil, err
		}
	}
	refs := make([]api.ProjectRef, 0, len(files))
	names := map[string][]string{}
	for _, file := range files {
		ref := api.ProjectRef{Name: strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)), ConfigPath: file, Key: canonical(file) + "\x00", ApplicationHome: paths.Home, ApplicationBin: paths.Bin, ExecutablePath: paths.Executable, SourceCommit: paths.SourceCommit, SourceDirty: paths.SourceDirty}
		ref.Presentation.ServiceInstruction = "steerline start-service -config " + file
		if runtime.GOOS == "windows" {
			ref.Presentation.ServiceInstruction = filepath.Join(paths.Home, "service-window.bat")
		}
		if err := connectionConfig(ctx, &ref); err != nil {
			ref.Error = err.Error()
		}
		refs = append(refs, ref)
		if ref.Identity != "" {
			names[ref.Identity] = append(names[ref.Identity], ref.Name)
		}
	}
	for i := range refs {
		if len(names[refs[i].Identity]) > 1 {
			refs[i].Conflict = "同仓库配置关联/冲突：" + strings.Join(names[refs[i].Identity], ", ") + "；控制操作已禁用"
		}
	}
	return refs, nil
}
func Prepare(ctx context.Context, configPath, runID string, paths Paths) ([]api.ProjectRef, console.Options, error) {
	refs, err := Load(ctx, configPath, paths)
	options := console.Options{RunID: runID, Clients: map[string]console.ConsoleClient{}, ApplicationHome: paths.Home, ApplicationBin: paths.Bin, ApplicationEtc: filepath.Join(paths.Home, "etc")}
	if err != nil {
		return nil, options, err
	}
	for i := range refs {
		if refs[i].Error != "" {
			continue
		}
		client := httpclient.NewConsole(ctx, refs[i])
		refs[i] = client.Project()
		options.Clients[refs[i].Key] = client
	}
	return refs, options, nil
}
func connectionConfig(ctx context.Context, ref *api.ProjectRef) error {
	var config struct {
		Repository       string `json:"repository"`
		RunDir           string `json:"run_dir"`
		TaskAuthority    string `json:"task_authority"`
		WorkflowProtocol string `json:"workflow_protocol"`
		Git              struct {
			Executable string   `json:"executable"`
			Args       []string `json:"args"`
		} `json:"git"`
	}
	data, err := os.ReadFile(ref.ConfigPath)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &config); err != nil {
		return err
	}
	if config.Repository == "" || config.TaskAuthority != "native" || config.WorkflowProtocol != "worker-flow-v2" {
		return fmt.Errorf("connection requires repository, native task authority and worker-flow-v2")
	}
	repo := config.Repository
	if !filepath.IsAbs(repo) {
		repo = filepath.Join(filepath.Dir(ref.ConfigPath), repo)
	}
	executable := config.Git.Executable
	if executable == "" {
		executable = "git"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := append(append([]string(nil), config.Git.Args...), "rev-parse", "--path-format=absolute", "--git-common-dir")
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = repo
	out, err := command.Output()
	if err != nil {
		return fmt.Errorf("resolve connection repository identity: %w", err)
	}
	identity := strings.TrimSpace(string(out))
	if !filepath.IsAbs(identity) {
		identity = filepath.Join(repo, identity)
	}
	identity, err = filepath.EvalSymlinks(identity)
	if err != nil {
		return err
	}
	ref.Identity = canonical(identity)
	name := filepath.Base(identity)
	if strings.EqualFold(name, ".git") {
		name = filepath.Base(filepath.Dir(identity))
	}
	name = strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(name, "-"), "-.")
	if name == "" {
		name = "repo"
	}
	digest := sha256.Sum256([]byte(ref.Identity))
	ref.RunDir = config.RunDir
	if ref.RunDir == "" {
		ref.RunDir = filepath.Join(ref.ApplicationHome, "state", name+"-"+hex.EncodeToString(digest[:])[:12])
	} else if !filepath.IsAbs(ref.RunDir) {
		ref.RunDir = filepath.Join(filepath.Dir(ref.ConfigPath), ref.RunDir)
	}
	ref.RunDir = filepath.Clean(ref.RunDir)
	ref.Key = canonical(ref.ConfigPath) + "\x00" + canonical(ref.RunDir)
	ref.Presentation.NativeWorkflow = true
	ref.Presentation.TaskAuthority = config.TaskAuthority
	ref.Presentation.WorkflowProtocol = config.WorkflowProtocol
	return nil
}
