package httpapi

import (
	"context"
	"net/http"
)

// Identity returns the immutable routing binding, like the former native client.
// The public identity read separately checks committed runtime readiness.
func (s *Server) Identity(context.Context) Identity { return s.discoveryIdentity() }
func (s *Server) Command(ctx context.Context, envelope CommandEnvelope) (CommandResponse, error) {
	if err := envelope.Validate(ProjectID(s.options.ProjectIdentity), s.options.InstanceID); err != nil {
		return CommandResponse{}, &protocolError{status: 409, code: "stale_target", reason: err.Error()}
	}
	if envelope.Action == "stop_service" {
		return CommandResponse{}, &protocolError{status: 501, code: "unsupported", reason: "project stop_service is retired; use application stop"}
	}
	if envelope.Action == "open_native_session" {
		return s.commands.OpenNative(ctx, envelope)
	}
	return s.commands.Submit(ctx, envelope, currentActor())
}

// WriteError preserves the original error codes for both HTTP entry points.
func WriteProjectError(w http.ResponseWriter, err error) { writeProtocolError(w, err, "") }
