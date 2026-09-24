package steerline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const taskIdentityMapSchema = "task-identity-map/v1"

type TaskIdentityMap struct {
	Schema          string                `json:"schema"`
	ProjectIdentity string                `json:"project_identity"`
	Mappings        []TaskIdentityMapping `json:"mappings"`
}
type TaskIdentityMapping struct {
	TaskID         string `json:"task_id"`
	SourceProvider string `json:"source_provider"`
	SourceProject  string `json:"source_project"`
	SourceTaskID   string `json:"source_task_id"`
}
type oldSourceIdentity struct{ provider, project, task string }

func parseTaskIdentityMap(path, project string) (*TaskIdentityMap, string, map[oldSourceIdentity]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		b = nil
	} else if err != nil {
		return nil, "", nil, err
	}
	m := &TaskIdentityMap{}
	bySource := map[oldSourceIdentity]string{}
	byID := map[string]bool{}
	digest := ""
	if len(b) == 0 {
		return m, digest, bySource, nil
	}
	digest = identityMapDigest(b)
	if err = decode(b, m); err != nil {
		return nil, "", nil, fmt.Errorf("invalid task identity map: %w", err)
	}
	if m.Schema != taskIdentityMapSchema || m.ProjectIdentity != project {
		return nil, "", nil, errors.New("task identity map schema/project identity mismatch")
	}
	for _, entry := range m.Mappings {
		if !canonicalDecimalID(entry.TaskID) || entry.SourceProvider != "beads" || entry.SourceProject == "" || entry.SourceTaskID == "" {
			return nil, "", nil, errors.New("task identity map entry requires a canonical nonnegative decimal task ID and verified Beads source triple")
		}
		source := oldSourceIdentity{entry.SourceProvider, entry.SourceProject, entry.SourceTaskID}
		if _, ok := bySource[source]; ok {
			return nil, "", nil, errors.New("task identity map maps one Beads source to multiple task IDs")
		}
		if byID[entry.TaskID] {
			return nil, "", nil, errors.New("task identity map reuses a numeric task ID")
		}
		bySource[source] = entry.TaskID
		byID[entry.TaskID] = true
	}
	return m, digest, bySource, nil
}

func resolveDomainTaskIdentity(repository, runDir, project, sourceProject, sourceTask string) (string, string, error) {
	_, digest, bySource, err := parseTaskIdentityMap(filepath.Join(runDir, "task-identity-map.json"), project)
	if err != nil {
		return "", "", err
	}
	source := oldSourceIdentity{provider: "beads", project: sourceProject, task: sourceTask}
	if mapped := bySource[source]; mapped != "" {
		return mapped, digest, nil
	}
	known, err := readPlannedTaskIDs(repository)
	if err != nil {
		return "", "", err
	}
	if canonicalDecimalID(sourceTask) && known[sourceTask] {
		return "", "", fmt.Errorf("Beads task %q matches a planned numeric ID; add a verified task-identity-map/v1 entry before dispatch", sourceTask)
	}
	return "legacy:" + shortHash(project+"\x00beads\x00"+sourceProject+"\x00"+sourceTask), digest, nil
}

func canonicalDecimalID(s string) bool {
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)$`).MatchString(s) {
		return false
	}
	_, err := strconv.ParseUint(s, 10, 63)
	return err == nil
}
func identityMapDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readPlannedTaskIDs(repository string) (map[string]bool, error) {
	ids := map[string]bool{}
	dir := filepath.Join(repository, "docs", "roadmap", "task-ids")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return ids, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var value struct {
			ID uint64 `json:"id"`
		}
		data, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal(data, &value); e != nil {
			return nil, fmt.Errorf("invalid planned task ID file %s: %w", entry.Name(), e)
		}
		ids[strconv.FormatUint(value.ID, 10)] = true
	}
	return ids, nil
}
