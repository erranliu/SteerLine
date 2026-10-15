package server

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"os"
	"path/filepath"
)

func LauncherDirectory(home string) string { return filepath.Join(home, "ui", "web") }
func OpeningURL(ctx context.Context, home, project string) (string, error) {
	client, err := localservice.Open(ctx, home, LauncherDirectory(home), contract.WebProtocol, "/native/identity")
	if err != nil {
		return "", err
	}
	defer client.Close()
	var ticket struct {
		Ticket     string `json:"ticket"`
		HomeID     string `json:"home_id"`
		InstanceID string `json:"instance_id"`
	}
	if err = client.Request(ctx, "POST", "/auth/issue", nil, &ticket); err != nil {
		return "", err
	}
	if ticket.HomeID != client.Identity.HomeID || ticket.InstanceID != client.Identity.InstanceID || len(ticket.Ticket) != 64 {
		return "", errors.New("opening ticket identity mismatch")
	}
	route := "/"
	if project != "" {
		if len(project) != 64 {
			return "", errors.New("project ID must have 64 hex characters")
		}
		for _, c := range project {
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
				return "", errors.New("invalid project ID")
			}
		}
		route = "/projects/" + project + "/dag"
	}
	return "http://" + client.Identity.Address + route + "#ticket=" + ticket.Ticket, nil
}
func Stop(ctx context.Context, home string) error {
	return localservice.Stop(ctx, home, LauncherDirectory(home), contract.WebProtocol, "/native/identity", "/native/shutdown")
}
func LoadAddress(home string) (string, error) {
	var input struct {
		Version int    `json:"version"`
		Address string `json:"address"`
	}
	data, err := os.ReadFile(filepath.Join(home, "web.json"))
	if os.IsNotExist(err) {
		return "127.0.0.1:18742", nil
	}
	if err != nil {
		return "", err
	}
	if len(data) > 4096 {
		return "", errors.New("Web config too large")
	}
	if err = json.Unmarshal(data, &input); err != nil {
		return "", err
	}
	if input.Version != 1 {
		return "", errors.New("Web configuration version must be 1")
	}
	return input.Address, ValidateAddress(input.Address)
}
