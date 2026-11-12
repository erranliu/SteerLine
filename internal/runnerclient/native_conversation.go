package runnerclient

import (
	"context"
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

const NativeConversationHistory = "native-conversation-history"

type NativeConversationMessage struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Text       string `json:"text"`
	TurnID     string `json:"turn_id,omitempty"`
	BodyCursor string `json:"body_cursor,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type NativeConversationPage struct {
	NativeSessionTarget
	Messages   []NativeConversationMessage `json:"messages"`
	NextCursor string                      `json:"next_cursor,omitempty"`
	BodyOffset *int                        `json:"body_offset,omitempty"`
	Partial    bool                        `json:"partial"`
}

func (c Client) NativeConversation(ctx context.Context, target NativeSessionTarget, cursor string, limit int) (NativeConversationPage, error) {
	var page NativeConversationPage
	invalid := func() (NativeConversationPage, error) {
		return NativeConversationPage{}, &Failure{Code: "invalid_response", Message: "Original conversation page identity or contents could not be verified", Exit: 4}
	}
	if c.InstanceID == "" || target.RunID == "" || target.AttemptID == "" || target.SessionID == "" || target.Generation == "" || limit < 1 || limit > 50 || len(cursor) > 1024 {
		return NativeConversationPage{}, &Failure{Code: "invalid_arguments", Message: "Exact original Runner service, session and page bounds are required", Exit: 2}
	}
	args := []string{"--run-id", target.RunID, "--attempt-id", target.AttemptID, "--session-id", target.SessionID, "--generation", target.Generation, "--limit", strconv.Itoa(limit)}
	if cursor != "" {
		args = append(args, "--after", cursor)
	}
	response, err := c.Call(ctx, NativeConversationHistory, args, nil)
	if err != nil {
		return page, err
	}
	if json.Unmarshal(response.Data, &page) != nil || page.NativeSessionTarget != target ||
		page.Messages == nil || len(page.Messages) > limit || len(page.NextCursor) > 1024 ||
		(page.NextCursor != "" && page.NextCursor == cursor) {
		return invalid()
	}
	maxText := 4096
	if page.BodyOffset != nil {
		if cursor == "" || *page.BodyOffset < 0 || len(page.Messages) != 1 {
			return invalid()
		}
		maxText = 64 << 10
	}
	seen := make(map[string]bool)
	for _, message := range page.Messages {
		if message.ID == "" || seen[message.ID] || (message.Role != "user" && message.Role != "assistant") ||
			(message.Text == "" && message.BodyCursor == "") || len(message.Text) > maxText ||
			len(message.BodyCursor) > 1024 || (message.BodyCursor != "" && !message.Truncated) ||
			(page.BodyOffset != nil && message.Truncated) || !utf8.ValidString(message.Text) {
			return invalid()
		}
		seen[message.ID] = true
	}
	return page, nil
}
