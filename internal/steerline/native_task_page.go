package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type TaskDependencyStatus = task.TaskDependencyStatus

type NativeTaskError = task.Error

func nativeErr(code, message string) error { return &task.Error{Code: code, Message: message} }
func ValidateNativeTaskID(id string) error { return task.ValidateID(id) }

type NativeTaskCapabilities = contracts.NativeTaskCapabilities

type NativeTaskDetail = contracts.NativeTaskDetail

type NativeTaskPage = contracts.NativeTaskPage
