package discovery

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/web/server/nativequery"
)

func codexModels(ctx context.Context, executable string) (models []Model, version string, err error) {
	p, err := nativequery.Start(ctx, executable, "app-server")
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if e := p.Close(); e != nil {
			err = e
		}
	}()
	encoder := json.NewEncoder(p.Input)
	scanner := bufio.NewScanner(p.Output)
	scanner.Buffer(make([]byte, 4096), nativequery.Limit)
	count := 0
	id := 0
	call := func(method string, params any) (json.RawMessage, error) {
		id++
		if e := encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); e != nil {
			return nil, errors.New("Codex metadata connection failed")
		}
		for scanner.Scan() {
			raw := scanner.Bytes()
			count += len(raw)
			if count > nativequery.Limit {
				return nil, errors.New("Codex catalog exceeds 1 MiB")
			}
			var response struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(raw, &response) != nil {
				return nil, errors.New("invalid Codex metadata response")
			}
			if response.ID == nil {
				continue
			}
			if *response.ID != id || len(response.Error) > 0 || len(response.Result) == 0 {
				return nil, errors.New("Codex metadata request was rejected or mismatched")
			}
			return response.Result, nil
		}
		return nil, errors.New("Codex metadata query ended without a complete response")
	}
	init, e := call("initialize", map[string]any{"clientInfo": map[string]string{"name": "steerline-web-capabilities", "version": "1"}})
	if e != nil {
		return nil, "", e
	}
	var initialized struct {
		UserAgent string `json:"userAgent"`
	}
	json.Unmarshal(init, &initialized)
	version = initialized.UserAgent
	if e = encoder.Encode(map[string]any{"method": "initialized"}); e != nil {
		return nil, "", errors.New("Codex metadata handshake failed")
	}
	seen := map[string]bool{}
	cursor := ""
	modelIDs := map[string]bool{}
	for {
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, e := call("model/list", params)
		if e != nil {
			return nil, "", e
		}
		var page struct {
			Data []struct {
				ID            string `json:"id"`
				Model         string `json:"model"`
				DisplayName   string `json:"displayName"`
				Default       bool   `json:"isDefault"`
				Hidden        bool   `json:"hidden"`
				DefaultEffort string `json:"defaultReasoningEffort"`
				Efforts       []struct {
					Value       string `json:"reasoningEffort"`
					Description string `json:"description"`
				} `json:"supportedReasoningEfforts"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Data == nil {
			return nil, "", errors.New("Codex returned an invalid model catalog")
		}
		for _, m := range page.Data {
			if m.Hidden {
				continue
			}
			value := m.Model
			if value == "" {
				value = m.ID
			}
			if value == "" || modelIDs[value] {
				return nil, "", errors.New("Codex returned duplicate or empty model identities")
			}
			modelIDs[value] = true
			row := Model{ID: value, Label: m.DisplayName, Default: m.Default, DefaultEffort: m.DefaultEffort, Efforts: []Effort{}}
			if row.Label == "" {
				row.Label = value
			}
			efforts := map[string]bool{}
			for _, eff := range m.Efforts {
				if eff.Value == "" || efforts[eff.Value] {
					return nil, "", errors.New("Codex returned an invalid effort catalog")
				}
				efforts[eff.Value] = true
				row.Efforts = append(row.Efforts, Effort{eff.Value, eff.Description})
			}
			models = append(models, row)
		}
		if page.NextCursor == nil || *page.NextCursor == "" {
			break
		}
		cursor = *page.NextCursor
		if seen[cursor] {
			return nil, "", fmt.Errorf("Codex catalog pagination repeated a cursor")
		}
		seen[cursor] = true
	}
	if models == nil {
		models = []Model{}
	}
	return models, version, nil
}
