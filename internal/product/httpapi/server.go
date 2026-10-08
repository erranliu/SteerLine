package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
)

const maxCommandBytes = 1 << 20
const maxFinalReplyBytes = 4 << 20

type protocolError struct {
	status int
	code   string
	reason string
}

func (e *protocolError) Error() string { return e.reason }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !isLoopbackRequest(r.RemoteAddr) {
		writeError(w, http.StatusForbidden, "source_rejected", "local API accepts IPv4 loopback clients only", "")
		return
	}
	if r.Host != s.listener.Addr().String() {
		writeError(w, http.StatusMisdirectedRequest, "host_rejected", "Host must match the discovered loopback endpoint", "")
		return
	}
	if r.Header.Get("Origin") != "" {
		writeError(w, http.StatusForbidden, "origin_rejected", "browser Origin requests are not accepted", "")
		return
	}
	if !authorized(r.Header.Get("Authorization"), s.token) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "local API credential is missing or invalid", "")
		return
	}

	segments, err := pathSegments(r.URL.EscapedPath())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_path", err.Error(), "")
		return
	}
	if len(segments) == 2 && segments[0] == "v1" && segments[1] == "identity" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		identity := s.identityView(r.Context())
		writeJSON(w, http.StatusOK, identity)
		return
	}
	identity := s.discoveryIdentity()
	if len(segments) < 4 || segments[0] != "v1" || segments[1] != "projects" {
		writeError(w, http.StatusNotFound, "route_not_found", "route is not part of steerline-client/v1", "")
		return
	}
	if segments[2] != identity.ProjectID {
		writeError(w, http.StatusConflict, "project_mismatch", "route project_id does not match the authenticated service", "")
		return
	}
	if segments[3] == "commands" {
		if r.Method == http.MethodPost && !identity.Ready {
			// The only mutation permitted before readiness is a stop request for
			// this already discovered service instance.
			s.serveCommands(w, r, segments[4:])
			return
		}
		s.serveCommands(w, r, segments[4:])
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	s.serveRead(w, r, identity, segments[3:])
}

func (s *Server) discoveryIdentity() Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	identity := s.identity
	identity.Ready = identity.Ready && s.discovery.Ready && !s.closed
	if !identity.Ready && identity.Diagnostic == "" {
		identity.Diagnostic = diagnosticOrStatus(s.discovery.Diagnostic, s.discovery.Status)
	}
	return identity
}

func (s *Server) identityView(ctx context.Context) Identity {
	s.mu.RLock()
	identity := s.identity
	discoveryReady := s.discovery.Ready && !s.closed
	s.mu.RUnlock()
	if !discoveryReady {
		identity.Ready = false
		if identity.Status == "ready" {
			identity.Status = "starting"
		}
		return identity
	}
	snapshot := s.product.ReadProject(ctx, "", api.ProjectSnapshot{})
	connection := snapshot.Connection
	page := connection.Snapshot.Page
	if page == nil {
		identity.Ready = false
		identity.Status = string(connection.State)
		identity.Diagnostic = firstNonempty(connection.Diagnostic, connection.Snapshot.Error, snapshot.NativeTaskError, snapshot.HistoryError, "service runtime snapshot is unavailable")
		identity.SampledAt = time.Now().UTC()
		return identity
	}
	service := page.Service
	identity.SampledAt = connection.Snapshot.SampledAt
	identity.Capabilities = append([]string(nil), service.Capabilities...)
	if service.InstanceID != identity.InstanceID {
		identity.Ready, identity.Status = false, "stale-instance"
		identity.Diagnostic = "discovered service instance differs from the committed runtime instance"
		return identity
	}
	identity.Ready = service.Ready && connection.State == api.ConnectionConnected && connection.IdentityMatched && connection.ConfigMatched
	identity.Status = string(connection.State)
	identity.Diagnostic = firstNonempty(connection.Diagnostic, deref(service.Error))
	if identity.Ready {
		identity.Status, identity.Diagnostic = "ready", ""
	} else if identity.Diagnostic == "" {
		identity.Diagnostic = "the existing service is not ready for client commands"
	}
	return identity
}

func (s *Server) serveRead(w http.ResponseWriter, r *http.Request, identity Identity, path []string) {
	ctx := r.Context()
	queryValues := r.URL.Query()
	switch {
	case len(path) == 1 && path[0] == "tasks":
		limit, err := parseLimit(queryValues.Get("limit"), 50, 500)
		if err != nil {
			writeProtocolError(w, err, "")
			return
		}
		page, err := s.product.ListTasks(ctx, queryValues.Get("cursor"), limit)
		writeResult(w, page, err)
	case len(path) == 2 && path[0] == "tasks":
		page, err := s.product.ShowTask(ctx, path[1])
		writeResult(w, page, err)
	case len(path) == 1 && path[0] == "graph":
		limit, err := parseLimit(queryValues.Get("limit"), 100, api.MaxTaskGraphNodes)
		if err != nil {
			writeProtocolError(w, err, "")
			return
		}
		page, err := s.product.TaskGraph(ctx, queryValues.Get("cursor"), limit)
		writeResult(w, page, err)
	case len(path) == 1 && path[0] == "snapshot":
		snapshot := s.product.ReadProject(ctx, queryValues.Get("task_id"), api.ProjectSnapshot{})
		writeJSON(w, http.StatusOK, snapshot)
	case len(path) == 1 && path[0] == "candidates":
		candidates, err := s.product.TaskSnapshot(ctx)
		writeResult(w, candidates, err)
	case len(path) == 2 && path[0] == "runs":
		candidate, err := s.candidate(ctx, path[1])
		writeResult(w, candidate, err)
	case len(path) == 3 && path[0] == "runs" && path[2] == "activity":
		s.serveActivity(w, r, identity, path[1])
	case len(path) == 3 && path[0] == "runs" && path[2] == "conversation":
		s.serveConversation(w, r, identity, path[1])
	case len(path) == 3 && path[0] == "runs" && path[2] == "final":
		s.serveFinal(w, r, identity, path[1])
	case len(path) == 3 && path[0] == "runs" && path[2] == "body":
		s.serveBody(w, r, path[1])
	default:
		writeError(w, http.StatusNotFound, "route_not_found", "route is not part of steerline-client/v1", "")
	}
}

func (s *Server) candidate(ctx context.Context, runID string) (*api.CandidateSummary, error) {
	snapshot := s.product.ReadProject(ctx, "", api.ProjectSnapshot{})
	page := snapshot.Connection.Snapshot.Page
	if page == nil {
		return nil, &api.Error{Kind: api.Unavailable, Message: firstNonempty(snapshot.Connection.Diagnostic, "service runtime snapshot is unavailable")}
	}
	for _, run := range page.Runs {
		if run != nil && run.RunID == runID {
			copy := *run
			return &copy, nil
		}
	}
	return nil, &api.Error{Kind: api.UnknownRecord, Message: "execution run was not found in the current project snapshot"}
}

func (s *Server) serveActivity(w http.ResponseWriter, r *http.Request, identity Identity, runID string) {
	queryValues := r.URL.Query()
	candidate, err := s.candidate(r.Context(), runID)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	executionID, attemptID, err := exactRunIdentity(candidate, queryValues.Get("execution_id"), queryValues.Get("attempt_id"))
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	cursor := api.ActivityCursor{RunID: runID, ExecutionID: executionID, AttemptID: attemptID}
	if raw := queryValues.Get("cursor"); raw != "" {
		if err = decodeStrict([]byte(raw), &cursor); err != nil {
			writeError(w, http.StatusBadRequest, "cursor_invalid", err.Error(), "")
			return
		}
		if !cursor.Initialized || cursor.RunID != runID || cursor.ExecutionID != executionID || cursor.AttemptID != attemptID {
			writeError(w, http.StatusConflict, "cursor_invalidated", "activity cursor belongs to another execution or attempt; refresh from the beginning", "")
			return
		}
	}
	page, err := s.product.Activity(r.Context(), runID, executionID, attemptID, cursor)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, ActivityPage{ProjectID: identity.ProjectID, SampledAt: time.Now().UTC(), Page: page})
}

func (s *Server) serveConversation(w http.ResponseWriter, r *http.Request, identity Identity, runID string) {
	queryValues := r.URL.Query()
	candidate, err := s.candidate(r.Context(), runID)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	executionID, attemptID, err := exactRunIdentity(candidate, queryValues.Get("execution_id"), queryValues.Get("attempt_id"))
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	limit, err := parseLimit(queryValues.Get("limit"), 100, query.MaxEventPageSize)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	finalsOnly := queryValues.Get("finals_only") == "true"
	filter := api.ConversationFilter{WorkflowID: candidate.RunID, ExecutionID: executionID, AttemptID: attemptID,
		TurnID: queryValues.Get("turn_id"), FinalsOnly: finalsOnly}
	page, err := s.product.ConversationPage(r.Context(), filter, queryValues.Get("cursor"), limit)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, ConversationPage{ProjectID: identity.ProjectID, SampledAt: time.Now().UTC(), Page: page})
}

func (s *Server) serveFinal(w http.ResponseWriter, r *http.Request, identity Identity, runID string) {
	candidate, err := s.candidate(r.Context(), runID)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	if candidate.ExecutionID == "" || candidate.AttemptID == "" {
		writeError(w, http.StatusConflict, "final_unavailable", "run has no exact execution and attempt identity", "")
		return
	}
	filter := api.ConversationFilter{FinalsOnly: true, WorkflowID: candidate.RunID, ExecutionID: candidate.ExecutionID, AttemptID: candidate.AttemptID}
	page, err := s.product.ConversationPage(r.Context(), filter, "", query.MaxEventPageSize)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	reply := FinalReply{ProjectID: identity.ProjectID, WorkflowID: candidate.RunID, ExecutionID: candidate.ExecutionID,
		AttemptID: candidate.AttemptID, SampledAt: time.Now().UTC(), Page: page, Bodies: []FinalBody{}}
	var total int64
	for _, event := range page.Events {
		if event.Body == nil {
			continue
		}
		if total+event.Body.Bytes > maxFinalReplyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "response_too_large", "final reply bodies exceed the bounded response size; use conversation and body range routes", "")
			return
		}
		body, bodyErr := s.readFullBody(r.Context(), runID, *event.Body)
		if bodyErr != nil {
			writeProtocolError(w, bodyErr, "")
			return
		}
		total += int64(len(body.Text))
		reply.Bodies = append(reply.Bodies, body)
	}
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) serveBody(w http.ResponseWriter, r *http.Request, runID string) {
	candidate, err := s.candidate(r.Context(), runID)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	queryValues := r.URL.Query()
	encoded := queryValues.Get("ref")
	refBytes, err := hex.DecodeString(encoded)
	if err != nil || len(refBytes) > 16<<10 {
		writeError(w, http.StatusBadRequest, "body_reference_invalid", "body ref must be bounded hexadecimal encoded product JSON", "")
		return
	}
	var ref api.ConversationBodyRef
	if err = decodeStrict(refBytes, &ref); err != nil {
		writeError(w, http.StatusBadRequest, "body_reference_invalid", err.Error(), "")
		return
	}
	if ref.ExecutionID != candidate.ExecutionID || ref.AttemptID != candidate.AttemptID {
		writeError(w, http.StatusConflict, "target_mismatch", "body reference belongs to another execution or attempt", "")
		return
	}
	offset, err := parseInt64(queryValues.Get("offset"), 0, 1<<62)
	if err != nil {
		writeError(w, http.StatusBadRequest, "offset_invalid", err.Error(), "")
		return
	}
	limit, err := parseLimit(queryValues.Get("limit"), 256<<10, 256<<10)
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	page, err := s.product.Body(r.Context(), ref, offset, limit)
	writeResult(w, page, err)
}

func (s *Server) readFullBody(ctx context.Context, runID string, ref api.ConversationBodyRef) (FinalBody, error) {
	if ref.Bytes < 0 || ref.Bytes > 16<<20 {
		return FinalBody{}, &protocolError{status: http.StatusRequestEntityTooLarge, code: "body_too_large", reason: "final body exceeds the product body's bounded size"}
	}
	var text strings.Builder
	var offset int64
	for offset < ref.Bytes || offset == 0 && ref.Bytes == 0 {
		page, err := s.product.Body(ctx, ref, offset, 256<<10)
		if err != nil {
			return FinalBody{}, err
		}
		if page.Offset != offset || page.NextOffset <= offset && !page.Complete {
			return FinalBody{}, &protocolError{status: http.StatusConflict, code: "body_cursor_invalidated", reason: "message body page did not advance on its original reference"}
		}
		text.Write(page.Data)
		offset = page.NextOffset
		if page.Complete {
			return FinalBody{ItemKey: ref.ItemKey, Revision: ref.Revision, SHA256: page.SHA256, Completeness: page.Completeness, Text: text.String(), Complete: page.Complete}, nil
		}
		if ref.Bytes == 0 {
			break
		}
	}
	return FinalBody{ItemKey: ref.ItemKey, Revision: ref.Revision, SHA256: ref.SHA256, Completeness: ref.Completeness, Text: text.String(), Complete: false}, nil
}

func exactRunIdentity(candidate *api.CandidateSummary, executionID, attemptID string) (string, string, error) {
	if candidate == nil || candidate.ExecutionID == "" || candidate.AttemptID == "" {
		return "", "", &protocolError{status: http.StatusConflict, code: "target_unavailable", reason: "run has no exact execution and attempt identity"}
	}
	if executionID != "" && executionID != candidate.ExecutionID || attemptID != "" && attemptID != candidate.AttemptID {
		return "", "", &protocolError{status: http.StatusConflict, code: "target_mismatch", reason: "requested execution or attempt differs from the current run identity"}
	}
	return candidate.ExecutionID, candidate.AttemptID, nil
}

func pathSegments(path string) ([]string, error) {
	if path == "" || path[0] != '/' {
		return nil, errors.New("request path must be absolute")
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return nil, nil
	}
	segments := make([]string, len(parts))
	for i, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil || strings.Contains(decoded, "/") || strings.Contains(decoded, "\\") || decoded == "." || decoded == ".." {
			return nil, errors.New("request path contains an invalid encoded segment")
		}
		segments[i] = decoded
	}
	return segments, nil
}

func isLoopbackRequest(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.IsLoopback()
}

func authorized(header, token string) bool {
	scheme, value, ok := strings.Cut(header, " ")
	return ok && strings.EqualFold(scheme, "Bearer") && token != "" && subtle.ConstantTimeCompare([]byte(value), []byte(token)) == 1
}

func parseLimit(raw string, fallback, maximum int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		return 0, &protocolError{status: http.StatusBadRequest, code: "limit_invalid", reason: fmt.Sprintf("limit must be from 1 to %d", maximum)}
	}
	return value, nil
}

func parseInt64(raw string, fallback, maximum int64) (int64, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 || value > maximum {
		return 0, &protocolError{status: http.StatusBadRequest, code: "offset_invalid", reason: "offset must be a non-negative bounded integer"}
	}
	return value, nil
}

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeProtocolError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func writeProtocolError(w http.ResponseWriter, err error, requestID string) {
	var protocolErr *protocolError
	if errors.As(err, &protocolErr) {
		writeError(w, protocolErr.status, protocolErr.code, protocolErr.reason, requestID)
		return
	}
	var productErr *api.Error
	if errors.As(err, &productErr) {
		status := http.StatusServiceUnavailable
		if productErr.Kind == api.UnknownRecord {
			status = http.StatusNotFound
		} else if productErr.Kind == api.IdentityConflict {
			status = http.StatusConflict
		} else if productErr.Kind == api.Unsupported {
			status = http.StatusNotImplemented
		}
		writeError(w, status, string(productErr.Kind), productErr.Message, requestID)
		return
	}
	message := err.Error()
	code, status := "request_failed", http.StatusServiceUnavailable
	if strings.Contains(strings.ToLower(message), "cursor_invalidated") {
		code, status = "cursor_invalidated", http.StatusConflict
	} else if strings.Contains(strings.ToLower(message), "unsupported") {
		code, status = "unsupported", http.StatusNotImplemented
	} else if strings.Contains(strings.ToLower(message), "stale_version") || strings.Contains(strings.ToLower(message), "stale_revision") || strings.Contains(strings.ToLower(message), "stale_instance") {
		code, status = "stale_target", http.StatusConflict
	}
	writeError(w, status, code, message, requestID)
}

func writeError(w http.ResponseWriter, status int, code, reason, requestID string) {
	writeJSON(w, status, ErrorResponse{Code: code, Reason: reason, RequestID: requestID})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "HTTP method is not supported for this route", "")
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

var _ http.Handler = (*Server)(nil)
