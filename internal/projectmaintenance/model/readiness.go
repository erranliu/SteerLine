package model

import "fmt"

// Readiness is the project-wide admission result for new executions only.
type Readiness struct {
	Status   string   `json:"status"`
	Commit   string   `json:"commit"`
	Total    int      `json:"total"`
	Ready    int      `json:"ready"`
	Blockers []string `json:"blockers"`
}

// MaterialsReadiness consumes one material for every unfinished task, including
// an unprepared placeholder when that task has no saved material.
func MaterialsReadiness(commit string, materials []Material) Readiness {
	r := Readiness{Status: "ready", Commit: commit, Total: len(materials), Blockers: []string{}}
	for _, material := range materials {
		if material.Status == "ready" && material.Commit == commit && commit != "" && material.Digest != "" && len(material.TestPlan) > 0 {
			r.Ready++
		} else {
			r.Blockers = append(r.Blockers, fmt.Sprintf("任务 #%s 的材料与测试安排未就绪", material.TaskID))
		}
	}
	if len(r.Blockers) > 0 {
		r.Status = "not_ready"
	}
	return r
}
