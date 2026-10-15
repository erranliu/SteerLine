package projectconnection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func resolverFixture(t *testing.T, role string) api.ProjectRef {
	t.Helper()
	home := t.TempDir()
	t.Setenv("STEERLINE_RESOLVER_TEST_ROLE", role)
	t.Setenv("STEERLINE_RESOLVER_TEST_RELEASE", filepath.Join(home, "release"))
	raw, err := json.Marshal(map[string]any{
		"repository": home, "task_authority": "native", "workflow_protocol": "worker-flow-v2",
		"git": map[string]any{"executable": os.Args[0], "args": []string{"-test.run=^TestWebSwitchResolverChild$"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "project.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return api.ProjectRef{ConfigPath: path, ApplicationHome: home}
}

func TestWebSwitchResolverInheritedPipeIsBounded(t *testing.T) {
	ref := resolverFixture(t, "parent")
	release := os.Getenv("STEERLINE_RESOLVER_TEST_RELEASE")
	t.Cleanup(func() {
		os.WriteFile(release, nil, 0600)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(release + ".done"); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("inherited-pipe fixture did not exit")
	})
	done := make(chan error, 1)
	go func() { done <- Resolve(context.Background(), &ref) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("inherited stdout accepted as a complete Git response")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Git exit still waits for a descendant's inherited output pipe")
	}
}

func TestWebSwitchResolverRejectsInvalidOutput(t *testing.T) {
	for _, role := range []string{"empty", "oversize"} {
		t.Run(role, func(t *testing.T) {
			ref := resolverFixture(t, role)
			if err := Resolve(context.Background(), &ref); err == nil {
				t.Fatal("invalid Git output produced a project identity")
			} else if role == "oversize" && !strings.Contains(err.Error(), "64 KiB") {
				t.Fatalf("output was not bounded: %v", err)
			}
			if ref.Identity != "" {
				t.Fatal("invalid Git output retained an identity")
			}
		})
	}
}

func TestWebSwitchResolverChild(t *testing.T) {
	switch os.Getenv("STEERLINE_RESOLVER_TEST_ROLE") {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestWebSwitchResolverChild$")
		child.Env = append(os.Environ(), "STEERLINE_RESOLVER_TEST_ROLE=descendant")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "descendant":
		release := os.Getenv("STEERLINE_RESOLVER_TEST_RELEASE")
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(release); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.WriteFile(release+".done", nil, 0600)
		os.Exit(0)
	case "oversize":
		fmt.Print(strings.Repeat("x", 128<<10))
		os.Exit(0)
	case "empty":
		os.Exit(0)
	}
}
