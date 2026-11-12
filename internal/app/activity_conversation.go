package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerbinding"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type activityConversationTarget struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	RunID     string `json:"run_id,omitempty"`
	AttemptID string `json:"attempt_id,omitempty"`
	State     string `json:"state"`
	stateDir  string
	binding   runnerclient.NativeSessionTarget
}

type activityConversations struct {
	ActivityID string                         `json:"activity_id"`
	Targets    []activityConversationTarget   `json:"targets"`
	Diagnostic string                         `json:"diagnostic,omitempty"`
	Operation  *activityConversationOperation `json:"operation,omitempty"`
}

type activityConversationOperation struct {
	Label      string `json:"label"`
	RequestID  string `json:"request_id"`
	Status     string `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

func conversationTarget(key, label, runID string, run *runnercontract.Run, stateDir string) activityConversationTarget {
	target := activityConversationTarget{Label: label, RunID: runID, stateDir: stateDir, State: "unbound"}
	if run != nil && run.RunID == runID {
		e := run.Execution
		target.AttemptID = e.AttemptID
		target.binding = runnerclient.NativeSessionTarget{RunID: runID, AttemptID: e.AttemptID, SessionID: e.SessionID, Generation: e.Generation}
	}
	b := target.binding
	if b.RunID != "" && b.AttemptID != "" && b.SessionID != "" && b.Generation != "" {
		target.State = "available"
	}
	hash := sha256.Sum256([]byte(strings.Join([]string{key, runID, b.AttemptID, b.SessionID, b.Generation}, "\x00")))
	target.ID = hex.EncodeToString(hash[:])
	return target
}

func (t activityConversationTarget) state(cli *runnerclient.Client) string {
	if t.State != "available" {
		return t.State
	}
	// This query can read a retained Run after an approved build replacement in
	// the original namespace. NativeConversation fences the current instance
	// and build, and Runner verifies the original execution/session binding.
	if cli == nil || cli.InstanceID == "" || t.stateDir == "" || !runnerbinding.SamePath(t.stateDir, cli.StateDir) {
		return "unavailable"
	}
	if !slices.Contains(cli.FullBindingCommands, runnerclient.NativeConversationHistory) {
		return "unsupported"
	}
	return "available"
}

func (p *runtimeProjectPort) readActivityConversation(ctx context.Context, activity string, values url.Values) (any, error) {
	fail := func(status int, code, reason string) (any, error) {
		return nil, &httpclient.ClientError{Status: status, Code: code, Reason: reason}
	}
	session, cursor := values.Get("session"), values.Get("cursor")
	for key, entries := range values {
		if len(entries) != 1 || entries[0] == "" ||
			(key != "session" && key != "cursor") ||
			(key == "session" && len(session) != 64) ||
			(key == "cursor" && (session == "" || len(cursor) > 1024)) {
			return fail(400, "invalid_reading_query", "对话读取参数无效")
		}
	}
	project := p.config.Identity()
	source, id, err := query.DecodeActivityID(project, activity)
	if err != nil {
		return fail(404, "activity_not_found", "找不到原活动")
	}
	resolve := activityConversationSources[source]
	if resolve == nil {
		return fail(503, "reading_unavailable", "活动对话来源暂不可读")
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	list, err := resolve(ctx, p.store, project, id)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(404, "activity_not_found", "找不到原活动")
	}
	if err != nil {
		return fail(503, "reading_unavailable", "无法核实原活动的对话")
	}
	cli := p.config.runtimeRunner
	if session == "" {
		list.ActivityID = activity
		if list.Targets == nil {
			list.Targets = []activityConversationTarget{}
		}
		for i := range list.Targets {
			list.Targets[i].State = list.Targets[i].state(cli)
		}
		return list, nil
	}

	index := slices.IndexFunc(list.Targets, func(t activityConversationTarget) bool { return t.ID == session })
	if index < 0 {
		return fail(409, "conversation_changed", "原会话绑定已变化，请刷新对话")
	}
	target := list.Targets[index]
	switch target.state(cli) {
	case "unbound":
		return fail(409, "conversation_not_bound", "尚无可核实的原会话绑定")
	case "unsupported":
		return fail(501, "conversation_unsupported", "当前 Runner 构建没有公开原会话历史")
	case "unavailable":
		return fail(503, "reading_unavailable", "原 Runner 状态目录或当前实例暂不可读")
	}
	page, err := cli.NativeConversation(ctx, target.binding, cursor, 25)
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
		Target   string `json:"target_id"`
		runnerclient.NativeConversationPage
	}{activity, session, page}, nil
}
