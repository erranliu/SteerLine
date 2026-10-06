package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/client"
)

func TestCLIExecutionProfileIsBoundIntoStartTaskRequest(t *testing.T) {
	repository := t.TempDir()
	if output, err := exec.Command("git", "init", "-b", "main", repository).CombinedOutput(); err != nil {
		t.Fatalf("initialize repository: %v: %s", err, output)
	}
	host := NewProcessHost(30 * time.Second)
	directory, err := NewBuiltinDirectory(host)
	if err != nil {
		t.Fatal(err)
	}
	workerPayload, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: os.Args[0], WindowsSandbox: "unelevated"})
	hostPayload, _ := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "fixture-host", Shell: "cmd.exe", WaitTimeout: "1m"})
	config := Config{
		Repository: repository, Git: executionbackend.Command{Executable: "git"},
		ExecutionPolicy:   executionbackend.ExecutionPolicy{Filesystem: executionbackend.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"},
		PublicationPolicy: executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker},
		PR:                executionmodel.PRProviderConfig{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: os.Args[0]}},
		ExecutionProfiles: []execution.ExecutionProfile{{ID: "orca", Name: "Orca", Host: "orca", Agent: "codex", ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: workerPayload}, {ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: hostPayload}}}},
	}
	config.ProcessHost = host
	config.ExecutionDirectory = directory
	config.bindApplicationFiles("fixture", "", "accepted", true)
	config.RunDir = filepath.Join(t.TempDir(), "state")
	ref := api.ProjectRef{ConfigPath: "fixture"}
	resources, err := clientResources(context.Background(), config, ref)
	if err != nil {
		t.Fatal(err)
	}
	client, err := client.Open(context.Background(), resources.Project, func(context.Context, api.ProjectRef) (client.Resources, error) { return resources, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.StartTaskPayload(context.Background(), "101", ""); err == nil {
		t.Fatal("manual start silently selected a profile")
	}
	payload, err := client.StartTaskPayload(context.Background(), "101", "orca")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, selected, err := config.ResolveExecutionProfile("orca", payload.ExecutionProfileDigest)
	if err != nil || payload.TaskID != "101" || payload.ExecutionProfileID != selected.ID || selected.PublicationPolicy.Publisher != "worker" {
		t.Fatalf("CLI request lost its selected route: %+v / %v", payload, err)
	}
}
