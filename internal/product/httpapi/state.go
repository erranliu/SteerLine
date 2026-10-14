package httpapi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateStateDirectory verifies the files the loopback transport owns under
// the existing run directory. Application preflight calls it before service
// startup or native writer access so unknown entries or weakened ACLs are not
// silently ignored.
func ValidateStateDirectory(runDir, projectIdentity string) error {
	root := filepath.Join(runDir, "local-api")
	if err := verifyPrivateDirectory(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("validate local API state directory: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var discovery *Discovery
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		switch entry.Name() {
		case "commands", "credentials":
			if !entry.IsDir() {
				return fmt.Errorf("local API %s entry is not a directory", entry.Name())
			}
			if err = verifyPrivateDirectory(path); err != nil {
				return fmt.Errorf("verify local API %s permissions: %w", entry.Name(), err)
			}
			if entry.Name() == "credentials" {
				err = validateCredentialsDirectory(path)
			} else {
				err = validateCommandDirectory(path, projectIdentity)
			}
			if err != nil {
				return err
			}
		case "discovery.json":
			value, readErr := readDiscoveryFile(path)
			if readErr != nil {
				return readErr
			}
			if projectIdentity != "" && value.RepositoryIdentity != projectIdentity {
				return errors.New("local API discovery belongs to a different repository identity")
			}
			discovery = &value
		default:
			if !temporaryFileName(entry.Name()) {
				return fmt.Errorf("local API state contains an unsupported entry %q", entry.Name())
			}
			if err = verifyPrivateFile(path); err != nil {
				return fmt.Errorf("verify local API temporary entry: %w", err)
			}
		}
	}
	if discovery != nil && discovery.CredentialPath != "" {
		credentialRoot := filepath.Join(root, "credentials")
		if !requestIDPattern.MatchString(discovery.LeaseID) || filepath.Base(discovery.CredentialPath) != discovery.LeaseID+".token" ||
			!privatePath(credentialRoot, discovery.CredentialPath) {
			return errors.New("local API discovery credential path does not match its instance lease")
		}
		if err = verifyPrivateFile(discovery.CredentialPath); err != nil {
			return fmt.Errorf("verify local API discovery credential: %w", err)
		}
		secret, readErr := os.ReadFile(discovery.CredentialPath)
		if readErr != nil || len(secret) != 64 {
			if readErr == nil {
				readErr = errors.New("credential length is invalid")
			}
			return fmt.Errorf("local API discovery credential is unavailable: %w", readErr)
		}
	}
	return nil
}

func validateCredentialsDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return errors.New("local API credentials directory contains a nested directory")
		}
		file := filepath.Join(path, entry.Name())
		if temporaryFileName(entry.Name()) {
			if err = verifyPrivateFile(file); err != nil {
				return err
			}
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".token")
		if !strings.HasSuffix(entry.Name(), ".token") || !requestIDPattern.MatchString(name) {
			return fmt.Errorf("local API credentials contains an unsupported file %q", entry.Name())
		}
		if err = verifyPrivateFile(file); err != nil {
			return err
		}
		secret, readErr := os.ReadFile(file)
		if readErr != nil || len(secret) != 64 {
			if readErr == nil {
				readErr = errors.New("credential length is invalid")
			}
			return fmt.Errorf("local API credential is invalid: %w", readErr)
		}
	}
	return nil
}

func validateCommandDirectory(path, projectIdentity string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	records := newFileRequestRecordOwner(path)
	for _, entry := range entries {
		if entry.IsDir() {
			return errors.New("local API command directory contains a nested directory")
		}
		file := filepath.Join(path, entry.Name())
		if temporaryFileName(entry.Name()) {
			if err = verifyPrivateFile(file); err != nil {
				return err
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") || !requestIDPattern.MatchString(strings.TrimSuffix(entry.Name(), ".json")) {
			return fmt.Errorf("local API command directory contains an unsupported file %q", entry.Name())
		}
		record, readErr := records.Read(strings.TrimSuffix(entry.Name(), ".json"))
		if readErr != nil {
			return fmt.Errorf("read local API request record %s: %w", entry.Name(), readErr)
		}
		if projectIdentity != "" && record.Envelope.ProjectID != ProjectID(projectIdentity) {
			return fmt.Errorf("local API request record %s belongs to a different project", entry.Name())
		}
	}
	return nil
}

func temporaryFileName(name string) bool {
	return strings.HasPrefix(name, ".credential-") && strings.HasSuffix(name, ".tmp") ||
		strings.HasPrefix(name, ".discovery-") && strings.HasSuffix(name, ".tmp")
}
