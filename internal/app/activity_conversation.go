package app

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func (p *runtimeProjectPort) readCandidateConversation(ctx context.Context, activity string, values url.Values) (any, error) {
	fail := func(status int, code, reason string) (any, error) {
		return nil, &httpclient.ClientError{Status: status, Code: code, Reason: reason}
	}
	cursor := values.Get("cursor")
	if len(values) != 0 && (len(values) != 1 || len(values["cursor"]) != 1 || cursor == "" || len(cursor) > 1024) {
		return fail(400, "invalid_reading_query", "对话分页参数无效")
	}
	project := p.config.Identity()
	source, id, err := query.DecodeActivityID(project, activity)
	if err != nil || source != "task_candidate" {
		return fail(404, "activity_not_found", "此活动没有可定位的任务候选对话")
	}
	reader, ok := p.store.(interface {
		Candidate(context.Context, string, string) (runnerstate.Candidate, error)
	})
	if !ok {
		return fail(503, "reading_unavailable", "候选对话暂不可读")
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	candidate, err := reader.Candidate(ctx, project, id)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(404, "activity_not_found", "找不到原候选")
	}
	if err != nil {
		return fail(503, "reading_unavailable", "无法核实原候选")
	}
	if candidate.Projection == nil || candidate.RunID == "" {
		return fail(409, "conversation_not_bound", "候选尚未绑定原会话")
	}
	execution := candidate.Projection.Execution
	if execution.AttemptID == "" || execution.SessionID == "" || execution.Generation == "" {
		return fail(409, "conversation_not_bound", "候选尚未绑定原会话")
	}
	cli := p.config.runtimeRunner
	if cli == nil || cli.InstanceID == "" || candidate.StateDir == "" || !strings.EqualFold(filepath.Clean(cli.StateDir), filepath.Clean(candidate.StateDir)) {
		return fail(503, "reading_unavailable", "原 Runner 实例暂不可读")
	}
	if !slices.Contains(cli.FullBindingCommands, runnerclient.NativeConversationHistory) {
		return fail(501, "conversation_unsupported", "当前 Runner 构建没有公开原会话历史")
	}
	target := runnerclient.NativeSessionTarget{RunID: candidate.RunID, AttemptID: execution.AttemptID, SessionID: execution.SessionID, Generation: execution.Generation}
	page, err := cli.NativeConversation(ctx, target, cursor, 25)
	if err != nil {
		var failure *runnerclient.Failure
		if errors.As(err, &failure) {
			switch failure.Code {
			case "unsupported_capability":
				return fail(501, "conversation_unsupported", "原执行适配器没有公开原会话历史")
			case "stale_revision", "invalid_cursor":
				return fail(409, "conversation_changed", "原会话或分页位置已变化，请刷新对话")
			}
		}
		return fail(503, "reading_unavailable", "原 Runner 对话暂不可读，请重试")
	}
	return struct {
		Activity string `json:"activity_id"`
		runnerclient.NativeConversationPage
	}{activity, page}, nil
}
