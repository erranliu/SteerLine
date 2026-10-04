package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

type CommandService struct {
	Store       persistence.CommandStore
	Project     string
	Work        taskapp.WorkPort
	taskResults taskapp.ResultPort
}

// The app supplies the task work and result capabilities only to execution.
func NewCommandService(store persistence.CommandStore, project string, work taskapp.WorkPort, results taskapp.ResultPort) CommandService {
	return CommandService{Store: store, Project: project, Work: work, taskResults: results}
}

func (s *Service) commandService(project string) CommandService {
	return NewCommandService(s.commandStore, project, s.taskWork, s.taskResults)
}
