package environmentadaptation

import (
	"strconv"

	"github.com/erranliu/SteerLine/internal/activityconversation"
	"github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func Conversation(record model.Record) activityconversation.Facts {
	var facts activityconversation.Facts
	for _, round := range record.Rounds {
		key := strconv.Itoa(round.Number)
		facts.Runs = append(facts.Runs, activityconversation.Run{Key: key, Label: "检测轮次 " + key, RunID: round.RunID, Projection: round.Run, StateDir: record.Build.StateDir})
	}
	return facts
}
