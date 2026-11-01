//go:build e2e

package testenv

import "testing"

func TestEnvironmentAdmissionProbePreservesDeclaredProjectBasis(t *testing.T) {
	basis := map[string][]byte{"SteerlineFlow/testing/test-list.json": []byte("project checks"), "entry.ps1": []byte("project entry"), "environment.md": []byte("project requirements")}
	for _, path := range []string{"SteerlineFlow/testing/test-list.json", "entry.ps1", "environment.md"} {
		if validateEnvironmentProbeFiles(basis, map[string][]byte{path: []byte("probe replacement")}) == nil {
			t.Fatal("probe changed project basis", path)
		}
	}
	if validateEnvironmentProbeFiles(map[string][]byte{"SteerlineFlow/Testing.md": []byte("project requirements")}, map[string][]byte{"SteerlineFlow/testing.md": []byte("probe replacement")}) == nil {
		t.Fatal("Windows case alias changed project basis")
	}

	if err := validateEnvironmentProbeFiles(basis, map[string][]byte{"SteerlineFlow/testing/test-list.json": []byte("project checks"), "unrelated-probe.md": []byte("new")}); err != nil {
		t.Fatal(err)
	}
	if err := validateEnvironmentProbeFiles(map[string][]byte{"entry.ps1": []byte("project entry")}, map[string][]byte{"SteerlineFlow/testing/test-list.json": []byte(`{"tests":[]}`)}); err != nil {
		t.Fatal("project without catalogue rejected", err)
	}
}
