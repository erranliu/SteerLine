package testenv

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
	"github.com/google/uuid"
	"net/url"
	"path/filepath"
	"strings"
)

func validationPullsPath(provider, repository, base string) string {
	query := url.Values{"state": {"all"}}
	if provider == "https://github.com" {
		query.Set("per_page", "50")
		query.Set("base", base)
	} else {
		query.Set("limit", "50")
		query.Set("base_branch", base)
	}
	return "repos/" + repository + "/pulls?" + query.Encode()
}

type EnvironmentOption struct {
	ID              string `json:"id"`
	Key             string `json:"environment_key"`
	Status          string `json:"status"`
	CurrentRevision uint64 `json:"current_revision"`
}

func (p *Project) EnvironmentOptions() []EnvironmentOption {
	s := p.Scenario
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	var result struct {
		Profiles []EnvironmentOption `json:"profiles"`
	}
	s.Must(client.Get(s.Context, "/projects/"+string(project.Identity().ProjectID)+"/environment/options", &result))
	return result.Profiles
}
func (p *Project) EnvironmentCommand(action string, payload any) httpcontract.CommandResponse {
	s := p.Scenario
	project, err := applicationclient.OpenProject(s.Context, p.Home, filepath.Base(p.Config))
	s.Must(err)
	defer project.Close()
	client, err := applicationclient.Open(s.Context, p.Home)
	s.Must(err)
	defer client.Close()
	identity := project.Identity()
	raw, err := json.Marshal(payload)
	s.Must(err)
	request := httpcontract.CommandEnvelope{RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""), ProjectID: identity.ProjectID, InstanceID: identity.InstanceID, Action: action, TargetID: string(identity.ProjectID), ExpectedVersion: 1, Payload: raw}
	var response httpcontract.CommandResponse
	err = client.Post(s.Context, "/projects/"+string(identity.ProjectID)+"/commands", request, &response)
	if err != nil {
		s.Must(client.Get(s.Context, "/projects/"+string(identity.ProjectID)+"/commands/"+request.RequestID, &response))
	}
	return response
}
