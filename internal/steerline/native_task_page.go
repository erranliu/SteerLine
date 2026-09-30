package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

const nativeTaskProtocol = "native-tasks/v1"

func canonicalDecimalID(value string) bool { return task.CanonicalDecimalID(value) }

type TaskDependencyStatus = task.TaskDependencyStatus

type nativeTaskCursor = query.TaskCursor
type NativeTaskError = task.Error

func nativeErr(code, message string) error { return &task.Error{Code: code, Message: message} }
func ValidateNativeTaskID(id string) error { return task.ValidateID(id) }

type NativeTaskCapabilities = contracts.NativeTaskCapabilities

type NativeTaskDetail = contracts.NativeTaskDetail

type NativeTaskPage = contracts.NativeTaskPage
