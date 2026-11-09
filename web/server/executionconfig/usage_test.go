package executionconfig

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUsageCollectionToggleSavesWithoutRemoteDiscovery(t *testing.T) {
	s, c := fixture(t)
	for _, enabled := range []bool{false, true} {
		base, err := s.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		c.UsagePolicy, _ = json.Marshal(map[string]bool{"enabled": enabled})
		saved, err := s.Save(context.Background(), base.SavedDigest, encode(t, s, c))
		if err != nil || string(saved.Config.UsagePolicy) != string(c.UsagePolicy) {
			t.Fatal(saved, err)
		}
	}
}
