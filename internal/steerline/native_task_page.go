package steerline

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func nativeErr(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }
