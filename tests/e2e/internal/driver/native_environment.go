package driver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Environment keeps native login/configuration at its existing external home.
// Controlled fixtures still receive their isolated empty Codex home.
func (c *Client) Environment(base []string) []string {
	if c.NativeHome == "" {
		return Environment(base, c.Home, c.StatePath)
	}
	var result []string
	count := 0
	for _, entry := range base {
		key, value, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "STEERLINE_") || upper == "CODEX_HOME" || upper == "TEMP" || upper == "TMP" || upper == "GOFLAGS" || upper == "GIT_TERMINAL_PROMPT" {
			continue
		}
		if upper == "GIT_CONFIG_COUNT" {
			n, err := strconv.Atoi(value)
			if err == nil && n >= 0 {
				count = n
			}
		}
		result = append(result, entry)
	}
	var fixture struct {
		GitURL     string `json:"git_url"`
		Repository string `json:"repository"`
	}
	if raw, err := os.ReadFile(c.StatePath); err == nil && json.Unmarshal(raw, &fixture) == nil && strings.HasPrefix(fixture.GitURL, "http://127.0.0.1:") {
		repository := fixture.Repository
		if repository == "" {
			if i := strings.Index(fixture.GitURL[7:], "/"); i >= 0 {
				repository = strings.TrimSuffix(fixture.GitURL[7+i+1:], ".git")
			}
		}
		if len(strings.Split(repository, "/")) == 2 {
			var clean []string
			for _, entry := range result {
				key, _, _ := strings.Cut(entry, "=")
				if !strings.EqualFold(key, "GIT_CONFIG_COUNT") {
					clean = append(clean, entry)
				}
			}
			result = append(clean, "GIT_CONFIG_COUNT="+strconv.Itoa(count+1), "GIT_CONFIG_KEY_"+strconv.Itoa(count)+"=url."+fixture.GitURL+".insteadOf", "GIT_CONFIG_VALUE_"+strconv.Itoa(count)+"=https://github.com/"+repository+".git")
		}
	}
	return append(result, "STEERLINE_HOME="+c.Home, "STEERLINE_E2E_CONTROLLED_STATE="+c.StatePath, "CODEX_HOME="+c.NativeHome, "TEMP="+filepath.Join(c.Home, "temp"), "TMP="+filepath.Join(c.Home, "temp"), "GIT_TERMINAL_PROMPT=0", "GOFLAGS=")
}
