package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	privatefs "github.com/erranliu/SteerLine/internal/product/httpfs"
)

type discovery struct {
	Version    int    `json:"version"`
	Address    string `json:"address"`
	ProjectID  string `json:"project_id"`
	InstanceID string `json:"instance_id"`
	Secret     string `json:"launcher_secret"`
}

func LauncherDirectory(home, projectIdentity string) string {
	return filepath.Join(home, "ui", "web", httpcontract.ProjectID(projectIdentity))
}

func writeDiscovery(dir string, value discovery) error {
	if err := privatefs.EnsurePrivateDirectory(dir); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := privatefs.CreatePrivateTemp(dir, ".discovery-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	return privatefs.ReplacePrivateFile(file.Name(), filepath.Join(dir, "discovery.json"))
}

// OpeningURL verifies current-user discovery and obtains a short-lived ticket.
// The long-lived launcher secret never leaves the native process or private file.
func OpeningURL(ctx context.Context, runDir, projectIdentity, address, launcherDir string) (string, error) {
	filePath := filepath.Join(launcherDir, "discovery.json")
	if err := privatefs.VerifyPrivateDirectory(filepath.Dir(filePath)); err != nil {
		return "", err
	}
	if err := privatefs.VerifyPrivateFile(filePath); err != nil {
		return "", err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return "", err
	}
	if len(data) > 65536 {
		return "", errors.New("Web discovery exceeds its bound")
	}
	var value discovery
	if err = json.Unmarshal(data, &value); err != nil {
		return "", err
	}
	if value.Version != 1 || value.ProjectID != httpcontract.ProjectID(projectIdentity) || value.InstanceID == "" || value.Address != address || ValidateAddress(value.Address) != nil || len(value.Secret) != 64 {
		return "", errors.New("Web discovery does not match the configured original project/address")
	}
	native, err := privatefs.ReadDiscoveryFile(filepath.Join(runDir, "local-api", "discovery.json"))
	if err != nil {
		return "", err
	}
	if native.ProjectID != value.ProjectID || native.InstanceID != value.InstanceID {
		return "", errors.New("Web discovery differs from the current native service instance")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+value.Address+"/auth/issue", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+value.Secret)
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Web opening redirects are forbidden") }}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("Web opening rejected (%d); check the original service", response.StatusCode)
	}
	var ticket struct {
		Ticket     string `json:"ticket"`
		ProjectID  string `json:"project_id"`
		InstanceID string `json:"instance_id"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&ticket); err != nil {
		return "", err
	}
	if ticket.ProjectID != value.ProjectID || ticket.InstanceID != value.InstanceID || len(ticket.Ticket) != 64 {
		return "", errors.New("Web opening response belongs to another instance")
	}
	return "http://" + value.Address + "/#ticket=" + ticket.Ticket, nil
}
