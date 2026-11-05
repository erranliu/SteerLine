package billingconfig

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/billing"
)

func draft() billing.Policy {
	return billing.Policy{Source: "synthetic", Currency: "USD", Basis: billing.Basis, Rules: []billing.Rule{{ID: "r", Provider: "p", Model: "m", From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Rates: billing.Rates{Input: "2", Read: "0.2", Write: "3", Output: "4"}}}}
}

func TestBillingConfigRejectsLinkedAuthority(t *testing.T) {
	s := Store{Home: t.TempDir()}
	base, err := s.Save("*", draft())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Home, "billing.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "protected.json")
	if err = os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(); err == nil {
		t.Fatal("hardlinked authority accepted")
	}
	if _, err = s.Save(base.Digest, draft()); err == nil {
		t.Fatal("hardlinked authority replaced")
	}
	after, err := os.ReadFile(alias)
	if err != nil || string(before) != string(after) {
		t.Fatal("protected bytes changed", err)
	}
}

func TestBillingConfigRejectsInvalidStoredFiles(t *testing.T) {
	for _, raw := range []string{`null`, `{"schema":"old"}`, `{"schema":"steerline-billing/v1","schema":"other"}`, strings.Repeat(" ", MaxBytes+1)} {
		s := Store{Home: t.TempDir()}
		base, err := s.Save("*", draft())
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(s.Home, "billing.json")
		if err = os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Read(); err == nil {
			t.Fatal("invalid file accepted")
		}
		if _, err = s.Save(base.Digest, draft()); err == nil {
			t.Fatal("invalid authority overwritten")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != raw {
			t.Fatal("invalid original lost", err)
		}
	}
}

func TestPublicationLimitIncludesFinalNewline(t *testing.T) {
	s := Store{Home: t.TempDir()}
	base, err := s.Save("*", draft())
	if err != nil {
		t.Fatal(err)
	}
	f := base.File
	f.Generation = 9
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f.Policies[0].Source += strings.Repeat("x", MaxBytes-1-len(raw))
	raw, err = json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if len(raw) != MaxBytes {
		t.Fatal(len(raw))
	}
	path := filepath.Join(s.Home, "billing.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Activate(before.Digest, before.Active)
	var failure *Error
	if !errors.As(err, &failure) || failure.Status != 413 {
		t.Fatal("oversize publication accepted", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || string(after) != string(raw) {
		t.Fatal("failed publication changed authority", readErr)
	}
	if _, err = s.Read(); err != nil {
		t.Fatal("original file became unreadable", err)
	}
}
func TestVersionsAndConditionalPublication(t *testing.T) {
	s := Store{Home: t.TempDir()}
	empty, err := s.Read()
	if err != nil || empty.Configured {
		t.Fatal(empty, err)
	}
	if _, err = os.Stat(filepath.Join(s.Home, "billing.json")); !os.IsNotExist(err) {
		t.Fatal("read created file")
	}
	if _, err = s.Save("", draft()); err == nil {
		t.Fatal("missing precondition")
	}
	one, err := s.Save("*", draft())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := one.ActivePolicy()
	if _, err = s.Save("*", draft()); err == nil {
		t.Fatal("overwrote create")
	}
	changed := draft()
	changed.Rules[0].Rates.Input = "9"
	two, err := s.Save(one.Digest, changed)
	if err != nil {
		t.Fatal(err)
	}
	if two.Generation != 2 || len(two.Policies) != 2 || two.Policies[0].Digest() != p.Digest() {
		t.Fatal("immutable history", two)
	}
	if _, err = s.Activate(one.Digest, p.Version); err == nil {
		t.Fatal("stale writer")
	}
	three, err := s.Activate(two.Digest, p.Version)
	if err != nil || three.Generation != 3 {
		t.Fatal(three, err)
	}
	active, _ := three.ActivePolicy()
	if active.Digest() != p.Digest() {
		t.Fatal("version changed")
	}
	before, _ := os.ReadFile(filepath.Join(s.Home, "billing.json"))
	bad := draft()
	bad.Rules[0].Rates.Input = "-1"
	if _, err = s.Save(three.Digest, bad); err == nil {
		t.Fatal("bad rate")
	}
	after, _ := os.ReadFile(filepath.Join(s.Home, "billing.json"))
	if string(before) != string(after) {
		t.Fatal("failed write changed file")
	}
}
func TestConditionalWritesAcrossProcesses(t *testing.T) {
	if home := os.Getenv("SL_BILLING_TEST_HOME"); home != "" {
		_, err := (Store{Home: home}).Save(os.Getenv("SL_BILLING_TEST_ETAG"), draft())
		if err == nil {
			os.Exit(0)
		}
		var e *Error
		if errors.As(err, &e) && e.Status == 412 {
			os.Exit(12)
		}
		os.Exit(13)
	}
	s := Store{Home: t.TempDir()}
	base, err := s.Save("*", draft())
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	children := []*exec.Cmd{}
	for i := 0; i < 2; i++ {
		c := exec.Command(exe, "-test.run=^TestConditionalWritesAcrossProcesses$")
		c.Env = append(os.Environ(), "SL_BILLING_TEST_HOME="+s.Home, "SL_BILLING_TEST_ETAG="+base.Digest)
		if err = c.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, c)
	}
	winners, conflicts := 0, 0
	for _, c := range children {
		err = c.Wait()
		if err == nil {
			winners++
		} else if c.ProcessState.ExitCode() == 12 {
			conflicts++
		} else {
			t.Fatal("child failed", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatal(winners, conflicts)
	}
}
