// Package billingconfig owns the application's versioned billing.json file.
package billingconfig

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/pathguard"
	"github.com/erranliu/SteerLine/internal/product/httpfs"
)

const Schema = "steerline-billing/v1"
const MaxBytes = 1 << 20

type File struct {
	Schema     string           `json:"schema"`
	Generation uint64           `json:"generation"`
	Active     string           `json:"active_policy_version"`
	Policies   []billing.Policy `json:"policies"`
}
type Snapshot struct {
	File
	Digest     string `json:"saved_digest"`
	Configured bool   `json:"configured"`
}
type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string                   { return e.Message }
func failure(status int, code, msg string) error { return &Error{status, code, msg} }

type Store struct{ Home string }

func (s Store) path() (string, error) {
	p := filepath.Join(s.Home, "billing.json")
	resolved, err := pathguard.ResolveProspective(p)
	absolute, _ := filepath.Abs(p)
	same := filepath.Clean(resolved) == filepath.Clean(absolute)
	if runtime.GOOS == "windows" {
		same = strings.EqualFold(filepath.Clean(resolved), filepath.Clean(absolute))
	}
	if err != nil || !httpfs.PrivatePath(s.Home, p) || !same {
		return "", failure(400, "invalid_path", "billing configuration path is linked or invalid")
	}
	return p, nil
}
func (s Store) Read() (Snapshot, error) {
	out := Snapshot{File: File{Schema: Schema, Policies: []billing.Policy{}}}
	p, err := s.path()
	if err != nil {
		return out, err
	}
	if err = pathguard.RejectHardLinkedFile(p); err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	f, err := httpfs.OpenPrivateSnapshot(p)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || len(raw) > MaxBytes {
		return out, failure(400, "billing_configuration_invalid", "billing.json is unreadable or too large")
	}
	if err = executionconfig.Decode(raw, &out.File); err != nil {
		return out, err
	}
	if err = out.File.Validate(); err != nil {
		return out, err
	}
	h := sha256.Sum256(raw)
	out.Digest = hex.EncodeToString(h[:])
	out.Configured = true
	return out, nil
}
func (f File) Validate() error {
	if f.Schema != Schema || f.Generation == 0 || len(f.Policies) == 0 || len(f.Policies) > 200 {
		return errors.New("invalid billing configuration version or size")
	}
	seen := map[string]bool{}
	found := false
	for _, p := range f.Policies {
		if err := p.Validate(); err != nil {
			return err
		}
		if seen[p.Version] {
			return errors.New("duplicate policy version")
		}
		seen[p.Version] = true
		found = found || p.Version == f.Active
	}
	if !found {
		return errors.New("active policy missing")
	}
	return nil
}
func (s Snapshot) ActivePolicy() (billing.Policy, error) {
	for _, p := range s.Policies {
		if p.Version == s.Active {
			return p, nil
		}
	}
	return billing.Policy{}, failure(400, "PRICE_POLICY_NOT_CONFIGURED", "未配置价表")
}
func ValidateDraft(p billing.Policy) error {
	p.ID = "billing"
	p.Version = "draft"
	p.CreatedAt = time.Now().UTC()
	return p.Validate()
}

// expected is "*" for create-only; all later writes require the saved ETag.
func (s Store) Save(expected string, p billing.Policy) (Snapshot, error) {
	if err := ValidateDraft(p); err != nil {
		return Snapshot{}, failure(400, "billing_policy_invalid", err.Error())
	}
	return s.change(expected, func(f *File) error {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return err
		}
		p.ID = "billing"
		p.Version = hex.EncodeToString(b[:])
		p.CreatedAt = time.Now().UTC()
		f.Policies = append(f.Policies, p)
		f.Active = p.Version
		return nil
	})
}
func (s Store) Activate(expected, version string) (Snapshot, error) {
	return s.change(expected, func(f *File) error {
		for _, p := range f.Policies {
			if p.Version == version {
				f.Active = version
				return nil
			}
		}
		return failure(404, "policy_not_found", "price version not found")
	})
}
func (s Store) change(expected string, update func(*File) error) (Snapshot, error) {
	if expected == "" {
		return Snapshot{}, failure(428, "precondition_required", "conditional configuration write required")
	}
	// Home also contains installed executables and project configuration. A
	// non-inheritable private directory ACL removes their inherited Windows
	// access. Only the billing file needs the protected private snapshot ACL.
	if err := os.MkdirAll(s.Home, 0700); err != nil {
		return Snapshot{}, err
	}
	var out Snapshot
	err := httpfs.WithConfigurationLock(s.Home, func() error {
		current, err := s.Read()
		if err != nil {
			return err
		}
		if expected == "*" && current.Configured || expected != "*" && (!current.Configured || strings.Trim(expected, "\"") != current.Digest) {
			return failure(412, "configuration_changed", "配置已变化，请重新读取")
		}
		f := current.File
		if f.Generation == math.MaxUint64 {
			return errors.New("generation exhausted")
		}
		if err = update(&f); err != nil {
			return err
		}
		f.Generation++
		if err = f.Validate(); err != nil {
			return err
		}
		raw, err := json.MarshalIndent(f, "", "  ")
		raw = append(raw, '\n')
		if err != nil || len(raw) > MaxBytes {
			return failure(413, "configuration_too_large", "billing configuration exceeds limit")
		}
		p, err := s.path()
		if err != nil {
			return err
		}
		temp, err := httpfs.CreatePrivateTemp(s.Home, ".billing-")
		if err != nil {
			return err
		}
		name := temp.Name()
		defer os.Remove(name)
		_, err = io.Copy(temp, bytes.NewReader(raw))
		if err == nil {
			err = temp.Sync()
		}
		err = errors.Join(err, temp.Close())
		if err != nil {
			return err
		}
		if err = httpfs.ReplacePrivateFile(name, p); err != nil {
			return err
		}
		// Return exactly the version committed by this transaction.
		h := sha256.Sum256(raw)
		out = Snapshot{File: f, Digest: hex.EncodeToString(h[:]), Configured: true}
		return nil
	})
	return out, err
}
