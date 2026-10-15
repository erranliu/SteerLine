package httpapi

import (
	"io"
	"net/http"
	"strings"
)

func (s *Server) serveCommands(w http.ResponseWriter, r *http.Request, path []string) {
	if len(path) == 0 {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.submitCommand(w, r)
		return
	}
	if len(path) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		s.readCommand(w, r, path[0])
		return
	}
	writeError(w, http.StatusNotFound, "route_not_found", "command route is not part of steerline-client/v1", "")
}

func (s *Server) submitCommand(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") == "" || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content_type_required", "POST /commands requires application/json", "")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCommandBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "request_body_failed", err.Error(), "")
		return
	}
	if len(body) == 0 || len(body) > maxCommandBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request_body_too_large", "command envelope must be between 1 byte and 1 MiB", "")
		return
	}
	var envelope CommandEnvelope
	if err = decodeStrict(body, &envelope); err != nil {
		writeError(w, http.StatusBadRequest, "request_invalid", err.Error(), envelope.RequestID)
		return
	}
	projectID := ProjectID(s.options.ProjectIdentity)
	if err = envelope.Validate(projectID, s.options.InstanceID); err != nil {
		writeProtocolError(w, &protocolError{status: http.StatusBadRequest, code: "request_invalid", reason: err.Error()}, envelope.RequestID)
		return
	}
	response, err := s.Command(r.Context(), envelope)
	if err != nil {
		writeProtocolError(w, err, envelope.RequestID)
		return
	}
	writeJSON(w, responseHTTPStatus(response.Status), response)
}

func (s *Server) readCommand(w http.ResponseWriter, r *http.Request, requestID string) {
	if !requestIDPattern.MatchString(requestID) {
		writeError(w, http.StatusBadRequest, "request_id_invalid", "request_id must be 32 lowercase hexadecimal characters", requestID)
		return
	}
	response, err := s.commands.Read(r.Context(), requestID)
	if err != nil {
		writeProtocolError(w, err, requestID)
		return
	}
	writeJSON(w, http.StatusOK, response)
}
