package developerconfig

import "testing"

func TestBuildPolicy(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatal(err)
	}
	if Resolve(Config{}).SkipEnvironmentReadiness() || (Policy{}).SkipEnvironmentReadiness() {
		t.Fatal("default configuration bypassed readiness")
	}
	configured := Config{SkipEnvironmentReadiness: true}
	if Resolve(configured).SkipEnvironmentReadiness() != Available {
		t.Fatal("build capability was not enforced")
	}
	if (configured.Validate() == nil) != Available {
		t.Fatal("release build accepted developer configuration")
	}
	policy := Resolve(configured)
	configured.SkipEnvironmentReadiness = false
	if policy.SkipEnvironmentReadiness() != Available {
		t.Fatal("resolved policy changed with its source")
	}
}
