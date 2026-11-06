package executionconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/erranliu/SteerLine/internal/developerconfig"
)

func TestDeveloperConfiguration(t *testing.T) {
	s, c := fixture(t)
	ctx := context.Background()
	base, err := s.Snapshot(ctx)
	if err != nil || base.DeveloperAvailable != developerconfig.Available || base.Config.Developer.SkipEnvironmentReadiness {
		t.Fatal(base, err)
	}
	c.Developer.SkipEnvironmentReadiness = true
	_, validateErr := s.Validate(ctx, encode(t, s, c))
	saved, saveErr := s.Save(ctx, base.SavedDigest, encode(t, s, c))
	if !developerconfig.Available {
		for _, err := range []error{validateErr, saveErr} {
			var rejected *Error
			if !errors.As(err, &rejected) || rejected.Code != "developer_disabled" {
				t.Fatal("release API accepted developer override", err)
			}
		}
		current, err := s.Current()
		if err != nil || current.Developer.SkipEnvironmentReadiness {
			t.Fatal("rejected override changed disk", current, err)
		}
		return
	}
	if validateErr != nil || saveErr != nil || !saved.DeveloperAvailable || !saved.Config.Developer.SkipEnvironmentReadiness || saved.SavedDigest == base.SavedDigest {
		t.Fatal(saved, validateErr, saveErr)
	}
	// Every project reads the same home-owned value, including after a new reader.
	current, err := New(s.Home).Current()
	if err != nil || !current.Developer.SkipEnvironmentReadiness || !sameValue(current.Profiles, c.Profiles) {
		t.Fatal(current, err)
	}
	if _, err := s.Save(ctx, base.SavedDigest, encode(t, s, c)); err == nil {
		t.Fatal("stale editor overwrote developer switches")
	}
	c.Developer.SkipEnvironmentReadiness = false
	reset, err := s.Save(ctx, saved.SavedDigest, encode(t, s, c))
	if err != nil || reset.Config.Developer.SkipEnvironmentReadiness {
		t.Fatal(reset, err)
	}
}
