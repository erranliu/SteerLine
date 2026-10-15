// Package applicationhttp routes only catalogued projects through their native
// in-process owners. Routing never initializes storage or starts a runtime.
package applicationhttp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"

	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

type ProjectPort interface {
	Identity(context.Context) httpcontract.Identity
	Read(context.Context, string, url.Values) (any, error)
	Command(context.Context, httpcontract.CommandEnvelope) (httpcontract.CommandResponse, error)
	Task(context.Context, string, string, json.RawMessage) (any, error)
}

type Directory interface {
	Catalog(context.Context) (contract.Catalog, error)
	Connect(context.Context, string) (ProjectPort, error)
}
type Server struct {
	Endpoint  *localservice.Endpoint
	Directory Directory
	Shutdown  context.CancelFunc
}

var taskReadRoute = regexp.MustCompile(`^/(task-batches/[0-9a-f]{32}|tasks/[^/]+/predecessors/requests/[0-9a-f]{32})$`)
var taskWriteRoute = regexp.MustCompile(`^/(task-batch-validation|task-batches|tasks/[^/]+/predecessors)$`)
var readRoute = regexp.MustCompile(`^/(identity|tasks|tasks/[^/]+|graph|snapshot|candidates|presentation|runs/[^/]+|runs/[^/]+/(activity|last-message|conversation|final|body|recovery|decision-eligibility)|commands/[0-9a-f]{32})$`)

func AllowedRead(route string) bool { return readRoute.MatchString(route) }
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.Endpoint.Authorized(r) {
		localservice.Error(w, 403, "source_rejected", "native application credential, exact loopback Host and no Origin required")
		return
	}
	if r.URL.RawPath != "" || strings.Contains(r.URL.Path, "..") || strings.ContainsAny(r.URL.Path, "\\%") || strings.Contains(r.URL.Path, "//") {
		localservice.Error(w, 400, "invalid_path", "canonical route required")
		return
	}
	root := "/v2/application"
	if r.URL.Path == root+"/identity" && r.Method == "GET" {
		localservice.JSON(w, 200, s.Endpoint.Identity)
		return
	}
	if r.URL.Path == root+"/shutdown" && r.Method == "POST" {
		s.Shutdown()
		localservice.JSON(w, 200, map[string]any{"status": "stopping", "identity": s.Endpoint.Identity})
		return
	}
	if r.URL.Path == root+"/projects" && r.Method == "GET" {
		catalog, err := s.Directory.Catalog(r.Context())
		if err != nil {
			localservice.BackendError(w, err)
		} else {
			localservice.JSON(w, 200, catalog)
		}
		return
	}
	path := strings.TrimPrefix(r.URL.Path, root+"/projects/")
	id, route, ok := strings.Cut(path, "/")
	route = "/" + route
	if !ok || len(id) != 64 || path == r.URL.Path || !(r.Method == "GET" && (AllowedRead(route) || taskReadRoute.MatchString(route)) || r.Method == "POST" && (route == "/commands" || taskWriteRoute.MatchString(route)) && r.URL.RawQuery == "") {
		localservice.Error(w, 404, "route_not_found", "route outside application protocol")
		return
	}
	c, err := s.Directory.Connect(r.Context(), id)
	if err != nil {
		localservice.BackendError(w, err)
		return
	}
	identity := c.Identity(r.Context())
	if identity.ProjectID != id {
		localservice.Error(w, 409, "project_mismatch", "route differs from native identity")
		return
	}
	var result any
	if taskReadRoute.MatchString(route) || taskWriteRoute.MatchString(route) {
		var data []byte
		if r.Method == "POST" {
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			defer reader.Close()
			data, err = io.ReadAll(reader)
			if err != nil {
				localservice.Error(w, 413, "request_body_too_large", err.Error())
				return
			}
		}
		result, err = c.Task(r.Context(), r.Method, route, json.RawMessage(data))
	} else if r.Method == "GET" {
		result, err = c.Read(r.Context(), route, r.URL.Query())
	} else {
		var envelope httpcontract.CommandEnvelope
		reader := http.MaxBytesReader(w, r.Body, 1<<20)
		defer reader.Close()
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&envelope); err != nil || decoder.Decode(new(any)) != io.EOF {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				localservice.Error(w, 413, "request_body_too_large", err.Error())
				return
			}
			localservice.Error(w, 400, "invalid_command", "one bounded command envelope required")
			return
		}
		if err = envelope.Validate(id, identity.InstanceID); err != nil {
			localservice.Error(w, 409, "stale_target", err.Error())
			return
		}
		result, err = c.Command(r.Context(), envelope)
	}
	if err != nil {
		httpapi.WriteProjectError(w, err)
		return
	}
	localservice.JSON(w, 200, result)
}
