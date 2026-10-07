package execution

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestClaimRechecksAcceptedConfigFiles(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	localGlobalExecutionFixture(t)
	projectPath := filepath.Join(t.TempDir(), "project.json")
	globalPath := fixtureGlobalExecutionConfigPath()
	project := []byte(`{"project_marker":{}}`)
	require(t, os.WriteFile(projectPath, project, 0600))
	global, err := os.ReadFile(globalPath)
	require(t, err)
	verifier := fixtureConfigVerifier{project: projectPath, global: globalPath, digest: fixtureLoadedConfigDigest(project, global)}
	require(t, verifyClaimConfigFiles(verifier))
	require(t, os.WriteFile(projectPath, append(project, ' '), 0600))
	if verifyClaimConfigFiles(verifier) == nil {
		t.Fatal("project drift accepted")
	}
	require(t, os.WriteFile(projectPath, project, 0600))
	require(t, os.WriteFile(globalPath, append(global, ' '), 0600))
	if verifyClaimConfigFiles(verifier) == nil {
		t.Fatal("global profile drift accepted")
	}
}

type fixtureConfigVerifier struct{ project, global, digest string }

func (v fixtureConfigVerifier) VerifyCurrentConfig() error {
	project, err := os.ReadFile(v.project)
	if err != nil {
		return err
	}
	global, err := os.ReadFile(v.global)
	if err != nil {
		return err
	}
	if fixtureLoadedConfigDigest(project, global) != v.digest {
		return nativeErr("stale_profile", "project or execution configuration changed")
	}
	return nil
}

func fixtureLoadedConfigDigest(project, global []byte) string {
	h := sha256.New()
	_, _ = h.Write(project)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(global)
	return fmt.Sprintf("%x", h.Sum(nil))
}
