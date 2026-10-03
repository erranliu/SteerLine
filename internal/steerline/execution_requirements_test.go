package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestExecutionRequirementsExistenceAndNonSecretEvidence(t *testing.T) {
	c := workerContextConfigFixture()
	c.Repository = t.TempDir()
	c.ExecutionRequirements = executionapi.ExecutionRequirements{Tools: []executionapi.ToolRequirement{{ID: "git"}}, Credentials: []executionapi.CredentialRequirement{{ID: "pr.token_env", Site: "service"}}}
	c.PR.TokenEnv = "STEERLINE_REQUIREMENTS_TOKEN"
	t.Setenv(c.PR.TokenEnv, "secret-requirement-token")
	e, err := evaluateExecutionRequirements(context.Background(), c)
	require(t, err)
	if e.Tools[0].Version == "" || e.Credentials[0].Status != executionapi.RequirementPresent {
		t.Fatalf("incomplete evidence: %+v", e)
	}
	data, err := json.Marshal(e)
	require(t, err)
	if strings.Contains(string(data), "secret-requirement-token") || strings.Contains(string(data), shortHash("secret-requirement-token")) {
		t.Fatal("secret or digest leaked into evidence")
	}
	c.ExecutionRequirements.Tools[0].ExactVersion = e.Tools[0].Version
	_, err = evaluateExecutionRequirements(context.Background(), c)
	require(t, err)
	c.ExecutionRequirements.Tools[0].ExactVersion = "different"
	if _, err = evaluateExecutionRequirements(context.Background(), c); err == nil {
		t.Fatal("wrong exact tool version accepted")
	}
	c.ExecutionRequirements.Tools = nil
	c.PR.TokenEnv = ""
	t.Setenv("GH_TOKEN", "other-host-login")
	if _, err = evaluateExecutionRequirements(context.Background(), c); err == nil {
		t.Fatal("host login substituted for absent provider reference")
	}
	c.PR.TokenEnv = "STEERLINE_REQUIREMENTS_TOKEN"
	t.Setenv(c.PR.TokenEnv, "")
	if _, err = evaluateExecutionRequirements(context.Background(), c); err == nil {
		t.Fatal("missing token reference accepted")
	}
	missing := queryHostRequirementTool(context.Background(), Config{Repository: c.Repository, Git: Command{Executable: filepath.Join(c.Repository, "missing-tool.exe")}}, "git")
	if missing.Status != executionapi.RequirementMissing {
		t.Fatal("missing tool not reported")
	}
	c.Git = Command{Executable: os.Args[0]} // Codex version is not a Git version.
	if got := queryHostRequirementTool(context.Background(), c, "git"); got.Status != executionapi.RequirementUnknown || got.Version != "" {
		t.Fatal("unparsed version became present", got)
	}
}

func TestExecutionRequirementsCannotInferOtherTargetEnvironment(t *testing.T) {
	for _, host := range []string{"orca", "orca_supervised"} {
		c := Config{Host: host, Agent: "codex"}
		orca := &orcaworker.ProfileConfig{Executable: "must-not-run", Shell: "cmd.exe", WaitTimeout: "1m"}
		if host == "orca_supervised" {
			remoteSettings := &remote.ProfileConfig{ConnectionID: "connection", EnvironmentID: "remote", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1}
			setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex"}, orca, remoteSettings)
		} else {
			setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex"}, orca)
		}
		c.ExecutionRequirements.Tools = []executionapi.ToolRequirement{{ID: "go"}}
		e, err := evaluateExecutionRequirements(context.Background(), c)
		if err == nil || len(e.Tools) != 1 || e.Tools[0].Status != executionapi.RequirementUnknown || e.Tools[0].Target != targetFor(c) {
			t.Fatalf("center environment proved another target: %+v %v", e, err)
		}
	}
}

func TestExecutionRequirementsDoNotUpgradeProbeToVerified(t *testing.T) {
	c := workerContextConfigFixture()
	c.Repository = t.TempDir()
	c.RunDir = t.TempDir()
	c.ExecutionRequirements.Capabilities = []string{"workspace_inspection"}
	if _, err := evaluateExecutionRequirements(context.Background(), c); err == nil {
		t.Fatal("version query became verified capability")
	}
	entry, err := registrationFor(c)
	require(t, err)
	report, err := entry.ProbeRequirements(context.Background(), c)
	require(t, err)
	evidence := filepath.Join(t.TempDir(), "native.txt")
	require(t, os.WriteFile(evidence, []byte("fixture evidence; not native certification"), 0600))
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "workspace_inspection" {
			report.Capabilities[i].Status = executionapi.CapabilityVerified
			report.Capabilities[i].Evidence = evidence + "#sha256=" + fmt.Sprintf("%x", sha256.Sum256([]byte("fixture evidence; not native certification")))
		}
	}
	require(t, (executionRecords{root: c.RunDir}).saveCapabilityReport(report))
	_, err = evaluateExecutionRequirements(context.Background(), c)
	require(t, err)
	require(t, os.WriteFile(evidence, []byte("changed evidence"), 0600))
	// Evidence loading checks the authenticated file reference, not a mock start.
	if _, err = evaluateExecutionRequirements(context.Background(), c); err == nil {
		t.Fatal("changed native evidence accepted")
	}
}

func TestExecutionRequirementsSnapshotCopiesAndRestoresOriginal(t *testing.T) {
	c := workerContextConfigFixture()
	c.Repository = t.TempDir()
	c.ExecutionRequirements.Tools = []executionapi.ToolRequirement{{ID: "git"}}
	e, err := evaluateExecutionRequirements(context.Background(), c)
	require(t, err)
	c.requirementsEvaluation = &e
	profile := c.projectExecutionProfile()
	fixed := profile.Freeze()
	if !testVerifyFixedExecutionProfile(fixed) {
		t.Fatal("valid evaluated snapshot rejected")
	}
	profile.ExecutionRequirements.Tools[0].ID = "go"
	profile.RequirementsEvaluation.Tools[0].Source = "changed"
	if fixed.ExecutionRequirements.Tools[0].ID != "git" || fixed.RequirementsEvaluation.Tools[0].Source == "changed" {
		t.Fatal("freeze aliases requirements or evidence")
	}
	current := c
	current.ExecutionRequirements.Tools = []executionapi.ToolRequirement{{ID: "python"}}
	current.Git.Executable = "must-not-run"
	restored := current.ApplyFixedExecutionProfile(fixed)
	if restored.ExecutionRequirements.Tools[0].ID != "git" || restored.Git.Executable != "git" {
		t.Fatal("restoration used current requirements or tools")
	}
	restored.requirementsEvaluation.Tools[0].Source = "other"
	restored.ExecutionRequirements.Tools[0].ID = "python"
	if fixed.RequirementsEvaluation.Tools[0].Source == "other" || fixed.ExecutionRequirements.Tools[0].ID != "git" {
		t.Fatal("restoration aliases original snapshot")
	}
	tampered := fixed
	tampered.Version = 3
	if testVerifyFixedExecutionProfile(tampered) {
		t.Fatal("old execution snapshot accepted")
	}
	tampered = fixed
	tampered.RequirementsEvidenceDigest = "other"
	if testVerifyFixedExecutionProfile(tampered) {
		t.Fatal("evidence digest drift accepted")
	}
}

func TestExecutionRequirementsClaimAndRestartBlockWithoutChangingSnapshot(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	c.ExecutionRequirements = executionapi.ExecutionRequirements{Tools: []executionapi.ToolRequirement{{ID: "git"}}, Credentials: []executionapi.CredentialRequirement{{ID: "pr.token_env", Site: "service"}}}
	c.PR.TokenEnv = "STEERLINE_CLAIM_REQUIREMENT_TOKEN"
	t.Setenv(c.PR.TokenEnv, "private-value")
	out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput()
	if err != nil {
		t.Fatalf("fixture push: %s %v", out, err)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	_, err = commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(8), nil), newID(), 1, "operator")
	require(t, err)
	authority, err := store.nativeAuthority(ctx, c.Identity())
	require(t, err)
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	require(t, store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}))
	options := TaskClaimOptions{TaskID: "8", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, Explicit: true, RequestedBy: "operator"}
	t.Setenv(c.PR.TokenEnv, "")
	if _, err = commands.ClaimNativeTask(ctx, c, options); err == nil {
		t.Fatal("missing required reference claimed task")
	}
	graph, err := store.taskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	if graph.ByID["8"].Status != "open" {
		t.Fatal("failed evaluation consumed task")
	}
	t.Setenv(c.PR.TokenEnv, "private-value")
	c.policySource = filepath.Join(t.TempDir(), "project.json")
	c.executionConfigPath = filepath.Join(t.TempDir(), "execution.json")
	projectBytes, globalBytes := []byte(`{"execution_requirements":{"tools":[{"id":"git"}]}}`), []byte(`{"execution_profiles":[]}`)
	require(t, os.WriteFile(c.policySource, projectBytes, 0600))
	require(t, os.WriteFile(c.executionConfigPath, globalBytes, 0600))
	claimProfileSettings := codexProfileSettings(c)
	c.configDigest = loadedConfigDigest(projectBytes, globalBytes)
	setTestCodexProfile(t, &c, claimProfileSettings)
	entry, err := registrationFor(c)
	require(t, err)
	fake := entry
	fake.QueryTool = func(ctx context.Context, config Config, id string) executionapi.RequirementEvidence {
		e := entry.QueryTool(ctx, config, id)
		require(t, os.WriteFile(c.policySource, append(projectBytes, ' '), 0600))
		return e
	}
	c.ExecutionDirectory = testDirectoryWithReplacement(t, c.ExecutionDirectory, fake)
	_, err = commands.ClaimNativeTask(ctx, c, options)
	if !isNativeErrorCode(err, "stale_profile") {
		t.Fatalf("configuration changed during evaluation but claim was accepted: %v", err)
	}
	graph, err = store.taskGraphSnapshot(ctx, c.Identity())
	require(t, err)
	if graph.ByID["8"].Status != "open" {
		t.Fatal("stale configuration consumed the task")
	}
	require(t, os.WriteFile(c.policySource, projectBytes, 0600))
	c.ExecutionDirectory = testDirectoryWithReplacement(t, c.ExecutionDirectory, entry)
	_, err = commands.ClaimNativeTask(ctx, c, options)
	require(t, err)
	s, err := newTestService(c)
	require(t, err)
	defer s.Close()
	run := s.find("8")
	if run == nil {
		t.Fatal("claimed run missing")
	}
	before := mustJSON(*s.workflowRecord(run).ExecutionProfile)
	t.Setenv(c.PR.TokenEnv, "")
	fixed, err := s.workerFlowContext(run)
	require(t, err)
	if err = s.checkCurrentExecutionRequirements(ctx, fixed); err == nil {
		t.Fatal("missing reference after restart accepted")
	}
	if string(before) != string(mustJSON(*s.workflowRecord(run).ExecutionProfile)) {
		t.Fatal("new blocking fact rewrote historical snapshot")
	}
	snapshot, err := store.Query(ctx, c.Identity())
	require(t, err)
	if snapshot.Executions[0].CurrentRequirementsError == nil || snapshot.Executions[0].CurrentRequirementsEvaluation == nil || snapshot.Executions[0].CurrentRequirementsEvaluation.Credentials[0].Status != executionapi.RequirementMissing {
		t.Fatal("current blocking fact was not persisted")
	}
}

func TestClaimRechecksAcceptedConfigFiles(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	localGlobalExecutionFixture(t)
	c := workerContextConfigFixture()
	c.policySource = filepath.Join(t.TempDir(), "project.json")
	c.executionConfigPath = fixtureGlobalExecutionConfigPath()
	project := []byte(`{"execution_requirements":{}}`)
	require(t, os.WriteFile(c.policySource, project, 0600))
	global, err := os.ReadFile(c.executionConfigPath)
	require(t, err)
	// Reuse the actual config-byte digest algorithm, including its delimiter.
	c.configDigest = loadedConfigDigest(project, global)
	require(t, verifyClaimConfigFiles(c))
	require(t, os.WriteFile(c.policySource, append(project, ' '), 0600))
	if verifyClaimConfigFiles(c) == nil {
		t.Fatal("project drift accepted")
	}
	require(t, os.WriteFile(c.policySource, project, 0600))
	require(t, os.WriteFile(c.executionConfigPath, append(global, ' '), 0600))
	if verifyClaimConfigFiles(c) == nil {
		t.Fatal("global profile drift accepted")
	}
}

func TestExecutionRegistryRejectsDuplicatesAndRequiresLifecyclePorts(t *testing.T) {
	c := workerContextConfigFixture()
	entry, err := registrationFor(c)
	require(t, err)
	if _, err = NewExecutionDirectory(c.ExecutionDirectory.Version(), testImplementationCodecs(), []ExecutionRegistration{entry, entry}); err == nil {
		t.Fatal("duplicate key accepted")
	}
	for _, cfg := range []Config{{ExecutionDirectory: c.ExecutionDirectory, Host: "unknown", Agent: "codex"}, {ExecutionDirectory: c.ExecutionDirectory, Host: "local", Agent: "unknown"}} {
		if _, err = registrationFor(cfg); err == nil {
			t.Fatal("unknown combination accepted")
		}
	}
	for _, registered := range c.ExecutionDirectory.entries {
		if registered.Lifecycle == nil {
			t.Fatal("registered implementation has no lifecycle port")
		}
	}
	incomplete := entry
	incomplete.Lifecycle = nil
	if _, err = NewExecutionDirectory(c.ExecutionDirectory.Version(), testImplementationCodecs(), []ExecutionRegistration{incomplete}); err == nil {
		t.Fatal("registration without a lifecycle port was accepted")
	}
}

func TestRequirementVersionParsers(t *testing.T) {
	for _, tc := range []struct{ id, output, version string }{
		{"git", "git version 2.51.0.windows.1", "2.51.0.windows.1"},
		{"go", "go version go1.26.4 windows/amd64", "1.26.4"},
		{"python", "Python 3.13.1", "3.13.1"},
		{"git", "codex-cli 0.159.2", ""}, {"go", "unknown", ""}, {"python", "Python version unknown", ""},
	} {
		matches := requirementToolPatterns[tc.id].FindStringSubmatch(tc.output)
		version := ""
		if len(matches) == 2 {
			version = matches[1]
		}
		if version != tc.version {
			t.Fatalf("%s parser: %q => %q", tc.id, tc.output, version)
		}
	}
}

func TestEmptyExecutionRequirementsKeepMandatoryChecks(t *testing.T) {
	c := workerContextConfigFixture()
	codex := codexProfileSettings(c)
	codex.Executable = "must-not-be-called"
	setTestCodexProfile(t, &c, codex)
	c.PublicationPolicy.Publisher = executionapi.PublisherWorker
	if err := requireWorkerFlowCapabilities(context.Background(), c); err == nil || !strings.Contains(err.Error(), "publication_policy") {
		t.Fatalf("empty requirements bypassed publisher check: %v", err)
	}
	c.PublicationPolicy = executionPublicationFixture()
	if err := requireWorkerFlowCapabilities(context.Background(), c); err == nil {
		t.Fatal("empty requirements bypassed mandatory CLI preflight")
	}
	c.Host = "orca_supervised"
	c.PublicationPolicy.Publisher = executionapi.PublisherWorker
	orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m"}
	remoteSettings := &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "remote", RepositorySelector: "id:repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1}
	codex = codexProfileSettings(c)
	codex.Model, codex.ReasoningEffort = "gpt-6-luna", "medium"
	codex.Profile, codex.WindowsSandbox = "", ""
	setTestImplementationProfiles(t, &c, codex, orca, remoteSettings)
	if err := requireWorkerFlowCapabilities(context.Background(), c); err == nil || !strings.Contains(err.Error(), "enforcement evidence") {
		t.Fatalf("empty requirements bypassed remote policy: %v", err)
	}
}

func TestRequirementToolsUseWorkerEnvironment(t *testing.T) {
	c := workerContextConfigFixture()
	c.Repository = t.TempDir()
	c.PR.TokenEnv = "PATH"
	// A service PATH that is excluded from the actual worker must not prove a
	// target tool exists. No query may use the service PATH as a substitute.
	for _, id := range []string{"go", "python"} {
		e := queryHostRequirementTool(context.Background(), c, id)
		if e.Status != executionapi.RequirementMissing {
			t.Fatalf("service environment proved %s after worker PATH removal: %+v", id, e)
		}
	}
}
