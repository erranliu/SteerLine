package taskrunner

import (
	"github.com/erranliu/SteerLine/internal/activityconversation"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func CandidateConversation(candidate runnerstate.Candidate) activityconversation.Facts {
	return activityconversation.Facts{
		Runs:       []activityconversation.Run{{Key: candidate.ID, Label: "候选执行", RunID: candidate.RunID, Projection: candidate.Projection, StateDir: candidate.RunnerBuild.StateDir}},
		Diagnostic: "这里只列出已核实会话绑定的执行；其他轮次的已保存内容可在已记录交互中查看。",
	}
}
