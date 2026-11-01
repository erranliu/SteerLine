package model

import "testing"

func TestProjectEntryOwnsOneCatalogueAndFinitePolicySources(t *testing.T) {
	entry, err := ParseProjectEntry([]byte(`{"version":1,"requirements":["quality/policy.md","docs/requirements.md"],"testing_catalogue":"quality/cases.json"}`))
	if err != nil || entry.TestingCatalogue != "quality/cases.json" || len(entry.Paths()) != 4 || entry.Requirements[0] != "docs/requirements.md" {
		t.Fatal(entry, err)
	}
	for _, body := range []string{
		`{"version":1,"requirements":[],"testing_catalogue":"quality/cases.json","requirements":[]}`,
		`{"version":1,"requirements":null,"testing_catalogue":"quality/cases.json"}`,
		`{"version":1,"requirements":["quality/policy.md","quality/policy.md"],"testing_catalogue":"quality/cases.json"}`,
		`{"version":1,"requirements":[],"testing_catalogue":"../outside.json"}`,
		`{"version":1,"requirements":[],"testing_catalogue":"C:/outside.json"}`,
		`{"version":1,"requirements":[],"testing_catalogue":"quality/*.json"}`,
		`{"version":1,"requirements":[],"testing_catalogue":"SteerlineFlow/environment-project.json"}`,
		`{"version":1,"requirements":["SteerlineFlow/environments/key/environment.md"],"testing_catalogue":"quality/cases.json"}`,
		`{"version":1,"requirements":[],"testing_catalogue":"quality/cases.json","required_checks":[]}`,
		`{"version":1,"requirements":["SteerlineProjectGuideDoc/environment-adaptation.md"],"testing_catalogue":"quality/cases.json"}`,
	} {
		if _, err := ParseProjectEntry([]byte(body)); err == nil {
			t.Fatalf("unsafe or duplicate entry accepted: %s", body)
		}
	}
}
