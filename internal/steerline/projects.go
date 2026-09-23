package steerline

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectInfo is one target repository configured under <STEERLINE_HOME>\etc.
type ProjectInfo struct {
	Name       string // config file name without .json
	ConfigPath string
	Repository string
	RunDir     string
	Err        error // set when the config cannot be loaded
}

// ListProjects enumerates <STEERLINE_HOME>\etc\*.json. A missing directory is an
// empty list, not an error.
func ListProjects() ([]ProjectInfo, error) {
	dir := SteerlineEtcDir()
	entries, e := os.ReadDir(dir)
	if e != nil {
		if os.IsNotExist(e) {
			return nil, nil
		}
		return nil, e
	}
	var out []ProjectInfo
	for _, en := range entries {
		name := en.Name()
		if en.IsDir() || !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		info := ProjectInfo{
			Name:       strings.TrimSuffix(name, filepath.Ext(name)),
			ConfigPath: filepath.Join(dir, name),
		}
		if c, e := LoadConfig(info.ConfigPath); e != nil {
			info.Err = e
		} else {
			info.Repository = c.Repository
			info.RunDir = c.RunDir
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ResolveProject picks a project by 1-based number or case-insensitive name.
func ResolveProject(projects []ProjectInfo, sel string) (ProjectInfo, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return ProjectInfo{}, fmt.Errorf("no project selected")
	}
	if n := atoiSafe(sel); n >= 1 && n <= len(projects) {
		return projects[n-1], nil
	}
	for _, p := range projects {
		if strings.EqualFold(p.Name, sel) {
			return p, nil
		}
	}
	return ProjectInfo{}, fmt.Errorf("project %q not found", sel)
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
