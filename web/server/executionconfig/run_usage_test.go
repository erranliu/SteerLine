package executionconfig

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestRunUsageConfigPreservation(t *testing.T) {
	s, c := fixture(t)
	base, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.UsagePolicy = json.RawMessage(`{"enabled":true,"remote_log_access":{"mode":"disabled","grants":[]}}`)
	saved, err := s.Save(context.Background(), base.SavedDigest, encode(t, s, c))
	if err != nil {
		t.Fatal(err)
	}
	saved.Config.Profiles[0].Name = "Renamed"
	_, err = s.Save(context.Background(), saved.SavedDigest, encode(t, s, saved.Config))
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.Current()
	var want, got any
	_ = json.Unmarshal(c.UsagePolicy, &want)
	_ = json.Unmarshal(current.UsagePolicy, &got)
	if err != nil || !reflect.DeepEqual(want, got) {
		t.Fatal("deployment telemetry settings lost", current, err)
	}
}
