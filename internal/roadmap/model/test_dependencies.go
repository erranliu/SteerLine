package model

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// IsPredecessor follows confirmed local dependencies, including transitive ones.
// A task cannot supply its own execution-time test arrangement.
func IsPredecessor(analysis *Analysis, provider, consumer string) bool {
	if analysis == nil || provider == "" || provider == consumer {
		return false
	}
	nodes := map[string]Node{}
	for _, n := range analysis.Tasks {
		nodes[n.Key] = n
	}
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(key string) bool {
		if seen[key] {
			return false
		}
		seen[key] = true
		for _, pred := range nodes[key].Proposed.Local {
			if pred == provider || visit(pred) {
				return true
			}
		}
		return false
	}
	return visit(consumer)
}

func TestProviders(r Record) (map[string]string, error) {
	newTasks := map[string]bool{}
	if r.Analysis != nil {
		for _, n := range r.Analysis.Tasks {
			newTasks[n.Key] = n.ExistingID == ""
		}
	}
	providers := map[string]string{}
	for _, d := range r.Documents {
		if !newTasks[d.Key] {
			return nil, fmt.Errorf("新增测试提供者不属于本批新任务：%s", d.Key)
		}
		for _, id := range d.ProvidesTests {
			if strings.TrimSpace(id) != id || id == "" || !utf8.ValidString(id) || providers[id] != "" {
				return nil, fmt.Errorf("任务 %s 的新增测试 ID 为空、无效或重复：%s", d.Key, id)
			}
			providers[id] = d.Key
		}
	}
	return providers, nil
}
