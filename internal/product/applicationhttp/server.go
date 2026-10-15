// Package applicationhttp routes only catalogued projects through their native
// HTTP owners. It never initializes project storage or starts a project service.
package applicationhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

type Directory interface {
	Catalog(context.Context) (contract.Catalog, error)
	Connect(context.Context, string) (*httpclient.Client, error)
}
type Server struct {
	Endpoint  *localservice.Endpoint
	Directory Directory
	Shutdown  context.CancelFunc
}

var readRoute = regexp.MustCompile(`^/(identity|tasks|tasks/[^/]+|graph|snapshot|candidates|presentation|runs/[^/]+|runs/[^/]+/(activity|conversation|final|body|recovery|decision-eligibility)|commands/[0-9a-f]{32})$`)

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
	root := "/v1/application"
	if r.URL.Path == root+"/identity" && r.Method == "GET" {
		localservice.JSON(w, 200, s.Endpoint.Identity)
		return
	}
	if r.URL.Path == root+"/shutdown" && r.Method == "POST" {
		localservice.JSON(w, 200, map[string]string{"status": "stopping"})
		s.Shutdown()
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
	if !ok || len(id) != 64 || path == r.URL.Path || !(r.Method == "GET" && AllowedRead(route) || r.Method == "POST" && route == "/commands" && r.URL.RawQuery == "") {
		localservice.Error(w, 404, "route_not_found", "route outside application protocol")
		return
	}
	c, err := s.Directory.Connect(r.Context(), id)
	if err != nil {
		localservice.BackendError(w, err)
		return
	}
	defer c.Close()
	identity := c.Identity()
	if identity.ProjectID != id {
		localservice.Error(w, 409, "project_mismatch", "route differs from native identity")
		return
	}
	if route == "/identity" {
		live, err := c.RefreshIdentity(r.Context())
		if err != nil {
			localservice.BackendError(w, err)
			return
		}
		localservice.JSON(w, 200, live)
		return
	}
	var result json.RawMessage
	if r.Method == "GET" {
		if r.URL.RawQuery != "" {
			route += "?" + r.URL.RawQuery
		}
		err = c.Get(r.Context(), route, &result)
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
		err = c.Post(r.Context(), route, envelope, &result)
	}
	if err != nil {
		localservice.BackendError(w, err)
		return
	}
	if len(result) == 0 {
		localservice.BackendError(w, errors.New("empty native response"))
		return
	}
	localservice.JSON(w, 200, result)
}
