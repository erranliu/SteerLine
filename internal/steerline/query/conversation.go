package query

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// ConversationService owns read rules without connection ownership or write capabilities.
type ConversationService struct {
	Project      string
	Events       ConversationEventReader
	Bodies       ConversationBodyReader
	Raw          ConversationRawReader
	Capabilities ConversationCapabilityReader
}

type conversationQueryCursor struct {
	Version int                          `json:"v"`
	Project string                       `json:"p"`
	Filter  contracts.ConversationFilter `json:"f"`
	After   int64                        `json:"n"`
}

func ConversationCursorAfter(project string, filter contracts.ConversationFilter, after int64) (string, error) {
	if project == "" || after < 0 {
		return "", errors.New("conversation cursor requires a project and non-negative event ID")
	}
	raw, err := json.Marshal(conversationQueryCursor{Version: 1, Project: project, Filter: filter, After: after})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (q ConversationService) ReadConversation(ctx context.Context, filter contracts.ConversationFilter, cursor string, limit int) (contracts.ConversationPage, error) {
	if q.Events == nil || q.Project == "" {
		return contracts.ConversationPage{}, errors.New("conversation query requires a store and project identity")
	}
	if limit < 1 {
		limit = DefaultEventPageSize
	}
	if limit > MaxEventPageSize {
		limit = MaxEventPageSize
	}
	after := int64(0)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return contracts.ConversationPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		var saved conversationQueryCursor
		if json.Unmarshal(raw, &saved) != nil || saved.Version != 1 || saved.Project != q.Project || saved.Filter != filter || saved.After < 0 {
			return contracts.ConversationPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		after = saved.After
	}
	page := contracts.ConversationPage{Events: []contracts.ConversationEvent{}}
	for len(page.Events) < limit {
		record, err := q.Events.NextConversationEvent(ctx, q.Project, filter, after)
		if err != nil {
			return contracts.ConversationPage{}, err
		}
		if record == nil {
			break
		}
		id, payloadBytes, payload := record.EventID, record.PayloadBytes, record.Payload
		if len(page.Events) > 0 && page.PayloadBytes+payloadBytes > MaxEventPageBytes {
			page.HasMore = true
			break
		}
		var event contracts.ConversationEvent
		if err = json.Unmarshal(payload, &event); err != nil {
			return contracts.ConversationPage{}, fmt.Errorf("decode conversation event %d: %w", id, err)
		}
		if err = event.Validate(); err != nil {
			return contracts.ConversationPage{}, fmt.Errorf("validate conversation event %d: %w", id, err)
		}
		event.EventID = id
		page.Events = append(page.Events, event)
		page.PayloadBytes += payloadBytes
		after = id
	}
	if len(page.Events) > 0 {
		if !page.HasMore && len(page.Events) == limit {
			probe, err := q.readOne(ctx, filter, after)
			if err != nil {
				return contracts.ConversationPage{}, err
			}
			page.HasMore = probe != nil
		}
		if page.HasMore {
			page.NextCursor, _ = ConversationCursorAfter(q.Project, filter, page.Events[len(page.Events)-1].EventID)
		}
	}
	return page, nil
}

func (q ConversationService) readOne(ctx context.Context, filter contracts.ConversationFilter, after int64) (*contracts.ConversationEvent, error) {
	record, err := q.Events.NextConversationEvent(ctx, q.Project, filter, after)
	if err != nil || record == nil {
		return nil, err
	}
	var event contracts.ConversationEvent
	if err := json.Unmarshal(record.Payload, &event); err != nil {
		return nil, err
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	event.EventID = record.EventID
	return &event, nil
}

func (q ConversationService) ReadMessageBody(ctx context.Context, ref contracts.ConversationBodyRef, offset int64, limit int) (contracts.ConversationBodyPage, error) {
	if q.Bodies == nil || q.Project == "" || ref.ItemKey == "" || ref.Revision < 1 || offset < 0 {
		return contracts.ConversationBodyPage{}, errors.New("message body reference or offset is invalid")
	}
	if limit < 1 {
		limit = 256 << 10
	}
	if limit > 256<<10 {
		limit = 256 << 10
	}
	if limit < utf8.UTFMax {
		limit = utf8.UTFMax
	}
	record, err := q.Bodies.ConversationBody(ctx, q.Project, ref, contracts.MaxConversationBodyBytes+1)
	if err != nil {
		return contracts.ConversationBodyPage{}, err
	}
	body, total, digest, complete := record.Body, record.TotalBytes, record.SHA256, string(record.Completeness)
	actual := sha256.Sum256(body)
	if len(body) > contracts.MaxConversationBodyBytes || total != int64(len(body)) || digest != hex.EncodeToString(actual[:]) || !utf8.Valid(body) {
		return contracts.ConversationBodyPage{}, errors.New("message body revision is corrupt")
	}
	if offset > total {
		return contracts.ConversationBodyPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	if digest != ref.SHA256 || total != ref.Bytes || contracts.BodyCompleteness(complete) != ref.Completeness {
		return contracts.ConversationBodyPage{}, errors.New("message body revision does not match the supplied reference")
	}
	end := offset + int64(limit)
	if end > total {
		end = total
	}
	data := body[offset:end]
	if len(data) > 0 && data[0]&0xC0 == 0x80 {
		return contracts.ConversationBodyPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	data = utf8SafePrefix(data)
	return contracts.ConversationBodyPage{ItemKey: ref.ItemKey, Revision: ref.Revision, Offset: offset, NextOffset: offset + int64(len(data)), TotalBytes: total, SHA256: digest, Completeness: contracts.BodyCompleteness(complete), Complete: offset+int64(len(data)) >= total, Data: data}, nil
}

func (q ConversationService) ReadRaw(ctx context.Context, ref contracts.ConversationRawRef, offset int64, limit int) (contracts.ConversationRawPage, error) {
	if q.Raw == nil || q.Project == "" || ref.ID == "" || ref.ExecutionID == "" || ref.AttemptID == "" || offset < 0 {
		return contracts.ConversationRawPage{}, errors.New("raw reference or offset is invalid")
	}
	if limit < 1 {
		limit = MaxEventPageBytes
	}
	if limit > MaxEventPageBytes {
		limit = MaxEventPageBytes
	}
	record, err := q.Raw.ConversationRaw(ctx, q.Project, ref, contracts.MaxConversationBodyBytes+1)
	if err != nil {
		return contracts.ConversationRawPage{}, err
	}
	body, total, digest := record.Data, record.TotalBytes, record.SHA256
	actual := sha256.Sum256(body)
	if len(body) > contracts.MaxConversationBodyBytes || total != int64(len(body)) || digest != hex.EncodeToString(actual[:]) {
		return contracts.ConversationRawPage{}, errors.New("raw conversation record is corrupt")
	}
	if offset > total {
		return contracts.ConversationRawPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	if ref.SHA256 != "" && (digest != ref.SHA256 || total != ref.Bytes) {
		return contracts.ConversationRawPage{}, errors.New("raw body does not match the supplied reference")
	}
	end := offset + int64(limit)
	if end > total {
		end = total
	}
	data := body[offset:end]
	return contracts.ConversationRawPage{RawID: ref.ID, Offset: offset, NextOffset: end, TotalBytes: total, SHA256: digest, Complete: end >= total, Data: data}, nil
}

func utf8SafePrefix(data []byte) []byte {
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return data
}

// ReadCapabilities preserves the shared backend/target report lookup; consumers select their source identity.
func (q ConversationService) ReadCapabilities(ctx context.Context, backend executionapi.BackendKind, protocol executionapi.ExecutionProtocol, targetID string, limit int) ([]contracts.ConversationCapabilityReport, error) {
	if q.Capabilities == nil {
		return nil, errors.New("conversation capability reader is required")
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	return q.Capabilities.ConversationCapabilityReports(ctx, backend, protocol, targetID, limit)
}
