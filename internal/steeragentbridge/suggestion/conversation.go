package suggestionbridge

import (
	"context"

	"github.com/erranliu/SteerLine/internal/activityconversation"
)

func Conversation(ctx context.Context, read func(context.Context, string, string) ([]byte, uint64, error), project, id string) (activityconversation.Facts, error) {
	record, err := loadRecord(ctx, read, project, id)
	if err != nil {
		return activityconversation.Facts{}, err
	}
	return activityconversation.Facts{Runs: []activityconversation.Run{{Key: id, Label: "生成续作提示词建议", RunID: record.RunID, Projection: record.Projection, StateDir: record.Runner.StateDir}}}, nil
}
