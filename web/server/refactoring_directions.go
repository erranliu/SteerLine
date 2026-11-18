package server

import (
	"encoding/json"
	"net/http"

	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
)

func (s *Server) refactoringDirections(w http.ResponseWriter, r *http.Request, csrf string) {
	var result json.RawMessage
	var err error
	if r.Method == "GET" {
		err = s.options.Client.Get(r.Context(), contract.RefactoringDirectionsPath, &result)
	} else {
		if !s.browserWrite(r) || !equal(r.Header.Get("X-CSRF-Token"), csrf) {
			fail(w, 403, "csrf_rejected", "same-origin JSON and session CSRF token required")
			return
		}
		var input contract.RefactoringDirectionChange
		if err := decode(w, r, &input, contract.RefactoringDirectionBodyLimit); err != nil {
			fail(w, 400, "invalid_direction", err.Error())
			return
		}
		err = s.options.Client.Post(r.Context(), contract.RefactoringDirectionsPath, input, &result)
	}
	if err != nil {
		backendError(w, err)
		return
	}
	writeJSON(w, 200, result)
}
