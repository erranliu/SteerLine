package execution

import (
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func isNativeErrorCode(err error, codes ...string) bool {
	var native *taskmodel.Error
	if !errors.As(err, &native) {
		var executionError *executionmodel.ResultError
		if !errors.As(err, &executionError) {
			return false
		}
		native = &taskmodel.Error{Code: executionError.Code, Message: executionError.Message}
	}
	for _, code := range codes {
		if native.Code == code {
			return true
		}
	}
	return false
}
