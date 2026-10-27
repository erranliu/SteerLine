package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeEnvironmentUsesExistingHomeWithoutCopyingCredentials(t *testing.T) {
	root := t.TempDir()
	native := filepath.Join(root, "existing-native")
	home := filepath.Join(root, "project")
	if err := os.MkdirAll(native, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("fixture-external-auth")
	if err := os.WriteFile(filepath.Join(native, "auth.json"), original, 0600); err != nil {
		t.Fatal(err)
	}
	c := Client{Home: home, NativeHome: native, StatePath: filepath.Join(root, "fixture.json")}
	got := strings.Join(c.Environment([]string{"Path=tools", "CODEX_HOME=other", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=external-helper", "GH_TOKEN=fixture-existing-env", "STEERLINE_HOME=installation"}), "\n")
	for _, want := range []string{"CODEX_HOME=" + native, "STEERLINE_HOME=" + home, "GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=external-helper", "GH_TOKEN=fixture-existing-env"} {
		if !strings.Contains(got, want) {
			t.Fatal("missing external native reference")
		}
	}
	if strings.Contains(got, "installation") || strings.Contains(got, "CODEX_HOME=other") {
		t.Fatal("inherited project binding")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatal("native environment created project auth/config files")
	}
	raw, err := os.ReadFile(filepath.Join(native, "auth.json"))
	if err != nil || string(raw) != string(original) {
		t.Fatal("external native authentication changed")
	}
}

func TestNativeEnvironmentPreservesOnlyConfiguredProviderReference(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "project.json")
	if err := os.WriteFile(config, []byte(`{"pr":{"token_env":"STEERLINE_GITEA_TOKEN"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := Client{Home: root, NativeHome: filepath.Join(root, "native"), Config: config}
	env := strings.Join(c.Environment([]string{"STEERLINE_GITEA_TOKEN=fixture-secret", "STEERLINE_OTHER_TOKEN=other", "STEERLINE_HOME=installation"}), "\n")
	if !strings.Contains(env, "STEERLINE_GITEA_TOKEN=fixture-secret") || strings.Contains(env, "STEERLINE_OTHER_TOKEN") || strings.Contains(env, "installation") {
		t.Fatal("configured provider reference lost or unrelated environment inherited")
	}
}
