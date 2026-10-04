package task

import (
	"github.com/erranliu/SteerLine/internal/task/model"
)

func newError(code, message string) error { return &model.Error{Code: code, Message: message} }
