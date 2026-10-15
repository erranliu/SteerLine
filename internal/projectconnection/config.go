package projectconnection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/pathguard"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func canonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
func Resolve(ctx context.Context, ref *api.ProjectRef) error {
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
	file, err := os.Open(ref.ConfigPath)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if len(data) > 1<<20 {
		return errors.New("project configuration exceeds 1 MiB")
	}
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
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := append(append([]string(nil), config.Git.Args...), "rev-parse", "--path-format=absolute", "--git-common-dir")
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = repo
	command.WaitDelay = 250 * time.Millisecond
	var output identityOutput
	command.Stdout = &output
	err = command.Run()
	if output.err != nil {
		err = output.err
	}
	if err != nil {
		return fmt.Errorf("resolve connection repository identity: %w", err)
	}
	identity := strings.TrimSpace(output.buffer.String())
	if identity == "" {
		return errors.New("resolve connection repository identity: Git returned an empty common-dir")
	}
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
	resolved, err := pathguard.ResolveProspective(ref.RunDir)
	if err != nil {
		return err
	}
	home, err := pathguard.ResolveProspective(ref.ApplicationHome)
	if err != nil {
		return err
	}
	if !pathguard.Within(home, resolved) {
		return errors.New("project run_dir must stay inside application home")
	}
	ref.Key = canonical(ref.ConfigPath) + "\x00" + canonical(ref.RunDir)
	ref.Presentation.NativeWorkflow = true
	ref.Presentation.TaskAuthority = config.TaskAuthority
	ref.Presentation.WorkflowProtocol = config.WorkflowProtocol
	return nil
}

type identityOutput struct {
	buffer bytes.Buffer
	err    error
}

func (b *identityOutput) Write(p []byte) (int, error) {
	if len(p) > (64<<10)-b.buffer.Len() {
		b.err = errors.New("Git common-dir output exceeds 64 KiB")
		return 0, b.err
	}
	return b.buffer.Write(p)
}
