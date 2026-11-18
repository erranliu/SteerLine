package applicationhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	ref "github.com/erranliu/SteerLine/internal/refactoring/model"
)

func (s *Server) refactoringDirections(w http.ResponseWriter, r *http.Request) {
	port, ok := s.Directory.(interface {
		RefactoringDirections(context.Context) (ref.DirectionCatalogue, error)
		ChangeRefactoringDirection(context.Context, ref.DirectionChange) (ref.Direction, error)
	})
	if !ok {
		localservice.Error(w, 503, "directions_unavailable", "重构方向管理暂不可用")
		return
	}
	if r.Method == "GET" {
		catalogue, err := port.RefactoringDirections(r.Context())
		if err != nil {
			localservice.BackendError(w, err)
			return
		}
		localservice.JSON(w, 200, catalogue)
		return
	}
	body := http.MaxBytesReader(w, r.Body, contract.RefactoringDirectionBodyLimit)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var input contract.RefactoringDirectionChange
	if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF {
		localservice.Error(w, 400, "invalid_direction", "重构方向请求必须包含一个有效的 JSON 对象")
		return
	}
	direction, err := port.ChangeRefactoringDirection(r.Context(), ref.DirectionChange(input))
	var invalid ref.DirectionInputError
	switch {
	case errors.Is(err, ref.ErrDirectionDuplicate), errors.Is(err, ref.ErrDirectionConflict):
		localservice.Error(w, 409, "direction_conflict", err.Error())
	case errors.Is(err, ref.ErrDirectionInvalid), errors.As(err, &invalid):
		localservice.Error(w, 400, "invalid_direction", err.Error())
	case err != nil:
		localservice.BackendError(w, err)
	default:
		localservice.JSON(w, 200, direction)
	}
}
