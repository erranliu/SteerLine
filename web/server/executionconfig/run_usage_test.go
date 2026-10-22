package executionconfig

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRunUsageConfigPreservation(t *testing.T) {
	s, c := fixture(t)
	base, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.PricingPolicy = filepath.Join(t.TempDir(), "price file.json")
	c.UsagePolicy = json.RawMessage(`{"enabled":true,"remote_log_access":{"mode":"disabled","grants":[]}}`)
	saved, err := s.Save(context.Background(), base.SavedDigest, encode(t, c))
	if err != nil {
		t.Fatal(err)
	}
	saved.Config.Profiles[0].Name = "Renamed"
	_, err = s.Save(context.Background(), saved.SavedDigest, encode(t, saved.Config))
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.Current()
	var want, got any
	_ = json.Unmarshal(c.UsagePolicy, &want)
	_ = json.Unmarshal(current.UsagePolicy, &got)
	if err != nil || current.PricingPolicy != c.PricingPolicy || !reflect.DeepEqual(want, got) {
		t.Fatal("deployment telemetry settings lost", current, err)
	}
}
