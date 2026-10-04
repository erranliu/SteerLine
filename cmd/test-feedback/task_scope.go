package main

// L1 includes the actual task rules, TM03 replay/result/dependency tests and
// fixed-document content rules. Git/SQLite/app/CLI wiring uses the Plan's
// directed integration tests; execution/shared scheduling stay in scoped.go.
var taskRulePackages = []string{
	"github.com/erranliu/SteerLine/internal/task/model",
	"github.com/erranliu/SteerLine/internal/task",
}
