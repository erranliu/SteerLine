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

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ConversationService owns read rules without connection ownership or write capabilities.
type ConversationService struct {
	Project      string
	Events       storageports.ConversationEventReader
	Bodies       storageports.ConversationBodyReader
	Capabilities storageports.ConversationCapabilityReader
}

func NewConversationService(store storageports.ConversationReader, project string) ConversationService {
	if store == nil {
		return ConversationService{Project: project}
	}
	return ConversationService{Project: project, Events: store, Bodies: store, Capabilities: store}
}

type conversationQueryCursor struct {
	Version int                    `json:"v"`
	Project string                 `json:"p"`
	Filter  api.ConversationFilter `json:"f"`
	After   int64                  `json:"n"`
}

func ConversationCursorAfter(project string, filter api.ConversationFilter, after int64) (string, error) {
	if project == "" || after < 0 {
		return "", errors.New("conversation cursor requires a project and non-negative event ID")
	}
	raw, err := json.Marshal(conversationQueryCursor{Version: 3, Project: project, Filter: filter, After: after})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (q ConversationService) ReadConversation(ctx context.Context, filter api.ConversationFilter, cursor string, limit int) (api.ConversationPage, error) {
	if filter.FinalsOnly && (filter.WorkflowID == "" || filter.ExecutionID == "") {
		return api.ConversationPage{}, errors.New("final replies require an exact workflow and execution")
	}
	if q.Events == nil || q.Project == "" {
		return api.ConversationPage{}, errors.New("conversation query requires a store and project identity")
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
			return api.ConversationPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		var saved conversationQueryCursor
		if json.Unmarshal(raw, &saved) != nil || saved.Version != 3 || saved.Project != q.Project || saved.Filter != filter || saved.After < 0 {
			return api.ConversationPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		after = saved.After
	}
	page := api.ConversationPage{Events: []api.ConversationEvent{}}
	for len(page.Events) < limit {
		record, err := q.Events.NextConversationEvent(ctx, q.Project, api.ConversationFilter(filter), after)
		if err != nil {
			return api.ConversationPage{}, err
		}
		if record == nil {
			break
		}
		id, payloadBytes, payload := record.EventID, record.PayloadBytes, record.Payload
		if len(page.Events) > 0 && page.PayloadBytes+payloadBytes > MaxEventPageBytes {
			page.HasMore = true
			break
		}
		var event executionmodel.ConversationEvent
		if err = json.Unmarshal(payload, &event); err != nil {
			return api.ConversationPage{}, fmt.Errorf("decode conversation event %d: %w", id, err)
		}
		if err = event.Validate(); err != nil {
			return api.ConversationPage{}, fmt.Errorf("validate conversation event %d: %w", id, err)
		}
		event.EventID = id
		projected, err := ConversationEvent(event)
		if err != nil {
			return api.ConversationPage{}, fmt.Errorf("project conversation event %d: %w", id, err)
		}
		page.Events = append(page.Events, projected)
		page.PayloadBytes += payloadBytes
		after = id
	}
	if len(page.Events) > 0 {
		if !page.HasMore && len(page.Events) == limit {
			probe, err := q.readOne(ctx, filter, after)
			if err != nil {
				return api.ConversationPage{}, err
			}
			page.HasMore = probe != nil
		}
		if page.HasMore {
			page.NextCursor, _ = ConversationCursorAfter(q.Project, filter, page.Events[len(page.Events)-1].EventID)
		}
	}
	return page, nil
}

func (q ConversationService) readOne(ctx context.Context, filter api.ConversationFilter, after int64) (*executionmodel.ConversationEvent, error) {
	record, err := q.Events.NextConversationEvent(ctx, q.Project, api.ConversationFilter(filter), after)
	if err != nil || record == nil {
		return nil, err
	}
	var event executionmodel.ConversationEvent
	if err := json.Unmarshal(record.Payload, &event); err != nil {
		return nil, err
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	event.EventID = record.EventID
	return &event, nil
}

func (q ConversationService) ReadMessageBody(ctx context.Context, ref api.ConversationBodyRef, offset int64, limit int) (api.ConversationBodyPage, error) {
	if q.Bodies == nil || q.Project == "" || ref.ItemKey == "" || ref.Revision < 1 || offset < 0 {
		return api.ConversationBodyPage{}, errors.New("message body reference or offset is invalid")
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
	record, err := q.Bodies.ConversationBody(ctx, q.Project, ref, executionmodel.MaxConversationBodyBytes+1)
	if err != nil {
		return api.ConversationBodyPage{}, err
	}
	body, total, digest, complete := record.Body, record.TotalBytes, record.SHA256, string(record.Completeness)
	actual := sha256.Sum256(body)
	if len(body) > executionmodel.MaxConversationBodyBytes || total != int64(len(body)) || digest != hex.EncodeToString(actual[:]) || !utf8.Valid(body) {
		return api.ConversationBodyPage{}, errors.New("message body revision is corrupt")
	}
	if offset > total {
		return api.ConversationBodyPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	if digest != ref.SHA256 || total != ref.Bytes || api.BodyCompleteness(complete) != ref.Completeness {
		return api.ConversationBodyPage{}, errors.New("message body revision does not match the supplied reference")
	}
	end := offset + int64(limit)
	if end > total {
		end = total
	}
	data := body[offset:end]
	if len(data) > 0 && data[0]&0xC0 == 0x80 {
		return api.ConversationBodyPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	data = utf8SafePrefix(data)
	return api.ConversationBodyPage{ItemKey: ref.ItemKey, Revision: ref.Revision, Offset: offset, NextOffset: offset + int64(len(data)), TotalBytes: total, SHA256: digest, Completeness: api.BodyCompleteness(complete), Complete: offset+int64(len(data)) >= total, Data: append([]byte(nil), data...)}, nil
}

func utf8SafePrefix(data []byte) []byte {
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return data
}

// ReadCapabilities preserves the shared backend/target report lookup; consumers select their source identity.
func (q ConversationService) ReadCapabilities(ctx context.Context, backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, targetID string, limit int) ([]api.ConversationCapabilityReport, error) {
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
