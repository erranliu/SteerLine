package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func nativeErr(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }

type NativeTaskCapabilities = contracts.NativeTaskCapabilities

type NativeTaskDetail = contracts.NativeTaskDetail

type NativeTaskPage = contracts.NativeTaskPage
