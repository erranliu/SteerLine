package codexapp

import (
	"reflect"
	"strings"
	"testing"
)

func TestLocalWindowsSandboxArgs(t *testing.T) {
	for _, tc := range []struct {
		name, goos, sandbox, mode string
		args, want                []string
		failure                   string
	}{
		{"fixed default", "windows", "workspace-write", "", []string{"--config", "example=true"}, []string{"--config", "example=true", "--config", `windows.sandbox="elevated"`}, ""},
		{"explicit elevated", "windows", "workspace-write", "elevated", nil, []string{"--config", `windows.sandbox="elevated"`}, ""},
		{"unsupported split roots", "windows", "workspace-write", "unelevated", nil, nil, "split writable roots"},
		{"read only unelevated", "windows", "read-only", "unelevated", nil, []string{"--config", `windows.sandbox="unelevated"`}, ""},
		{"other platform", "linux", "workspace-write", "", []string{"--config", "example=true"}, []string{"--config", "example=true"}, ""},
		{"full access unchanged", "windows", "danger-full-access", "", nil, nil, ""},
		{"unknown mode", "windows", "workspace-write", "auto", nil, nil, "windows_sandbox"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]string(nil), tc.args...)
			got, err := LocalSandboxArgs(tc.goos, tc.sandbox, tc.mode, tc.args)
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) || !reflect.DeepEqual(before, tc.args) {
				t.Fatalf("args=%v error=%v", got, err)
			}
		})
	}
	for _, args := range [][]string{
		{"--config", `windows.sandbox="unelevated"`}, {"-c", `windows.sandbox="elevated"`},
		{`--config=windows.sandbox="unelevated"`}, {`-cwindows.sandbox="unelevated"`},
		{"-c", `windows = { sandbox = "unelevated" }`}, {"-c", `windows."sandbox"="unelevated"`},
		{"-c", `'windows'.'sandbox'="unelevated"`}, {`-c=windows.sandbox="unelevated"`},
		{"--enable", "experimental_windows_sandbox"}, {"--disable=elevated_windows_sandbox"},
		{"--config", `features={experimental_windows_sandbox=true}`},
	} {
		if _, err := LocalSandboxArgs("windows", "workspace-write", "elevated", args); err == nil {
			t.Fatalf("accepted competing override: %v", args)
		}
	}
}
