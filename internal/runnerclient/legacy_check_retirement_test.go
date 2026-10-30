package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerstatepath"
)

func TestLegacyCheckRetirementPublishedContract(t *testing.T) {
	exe, version, sha := os.Getenv("STEERRUNNER_EXE"), os.Getenv("STEERRUNNER_VERSION"), os.Getenv("STEERRUNNER_SHA256")
	if exe == "" || version == "" || sha == "" {
		t.Skip("requires a pinned independent retired-Check Runner build")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	home := t.TempDir()
	dir := runnerstatepath.Directory(home)
	cli, release, err := (Selection{Executable: exe, Version: version, SHA256: sha}).Open(ctx, dir, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pinned Runner executable=%s version=%s sha256=%s resource_digest=%s state_dir=%s", exe, release.Version, sha, release.ResourceDigest, dir)
	// The static public command forbids service/state-dir options. Use its exact
	// offline argv against the executable already verified by Selection.Open.
	raw, err := exec.CommandContext(ctx, exe, "capabilities", "--offline", "--json", "--protocol", Protocol).Output()
	if err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(raw, &response); err != nil || response.Protocol != Protocol || !response.OK || response.Error != nil {
		t.Fatal("invalid offline capabilities envelope", err, response.Error)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"environment_check", "environment_check_schema", "environment_check_fixtures"} {
		if _, present := fields[name]; present {
			t.Fatal("retired public capability remains", name)
		}
	}
	service, err := cli.StartService(ctx, release)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer stopCancel()
		if err := cli.StopService(stopCtx, service.InstanceID, "legacy-retirement-stop-"+service.InstanceID); err != nil {
			t.Error(err)
			return
		}
		if err := cli.VerifyServiceSettlement(stopCtx, service.InstanceID, "legacy-retirement-stop-"+service.InstanceID); err != nil {
			t.Error(err)
			return
		}
		response, err := cli.Call(stopCtx, "service verify-stopped", nil, nil)
		if err != nil {
			t.Error(err)
			return
		}
		var stopped map[string]json.RawMessage
		if err := json.Unmarshal(response.Data, &stopped); err != nil {
			t.Error(err)
			return
		}
		if _, present := stopped["checks"]; present {
			t.Error("retired checks field remains in public stopped proof")
		}
		proof, err := cli.VerifyStopped(stopCtx)
		if err != nil {
			t.Error(err)
			return
		}
		if len(proof.Runs) != 0 {
			t.Error("empty contract check created a Run", proof)
			return
		}
		takeover, err := cli.VerifyTakeover(stopCtx)
		if err != nil {
			t.Error(err)
			return
		}
		if len(takeover.Items) != 0 {
			t.Error("empty contract check has takeover resources", takeover)
			return
		}
		if err := runnerstatepath.RemoveStopped(home); err != nil {
			t.Error(err)
		}
	}()
	bound := cli
	bound.InstanceID = service.InstanceID
	catalog, err := bound.Catalog(ctx, release)
	if err != nil {
		t.Fatal(err)
	}
	binding := catalog.Capabilities.InstanceBinding.FullBinding
	if binding == nil || binding.Contract != ServiceBindingContract || !binding.BeforeEffects || !binding.NoImplicitStart || len(binding.Commands) == 0 {
		t.Fatal("published full service binding is missing", binding)
	}
	for _, command := range binding.Commands {
		if strings.HasPrefix(command, "environment-check") {
			t.Fatal("retired command remains in public binding", command)
		}
	}
}
