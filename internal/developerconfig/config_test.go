package developerconfig

import "testing"

func TestBuildConfiguration(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatal(err)
	}
	if ((Config{SkipEnvironmentReadiness: true}).Validate() == nil) != Available {
		t.Fatal("release build accepted developer configuration")
	}
}
