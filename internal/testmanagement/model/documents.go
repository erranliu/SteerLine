package model

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

const CataloguePath = "docs/testing/test-list.json"
const TestingInstructions = "测试安排已固定。仅按 Plan 执行登记入口，不自动增加测试。提交被测代码后再执行需计当前通过的测试。运行内用入口已有 -out 参数将证据写到 .steerrunner/artifacts/<唯一执行ID>/ 新目录，目录须由项目 ignore 排除。入口程序输出 managed-*.json 及 summary.json，完整写入后原子发布 managed 报告；不要手写 Agent 自述测试结果。格式与定义摘要按项目测试规范；入口不支持时保留未完成。不得修改已固定测试定义或安排来制造通过。"

// TestReference admits only the optional documents derived from a validated
// contract Plan. It does not expand the TaskContract's arbitrary references.
func TestReference(doc runnercontract.Document, plan, commit string, ready bool) bool {
	if !strings.HasSuffix(plan, ".plan.md") {
		return false
	}
	if doc.Kind == "git" && doc.Content == nil && doc.Commit == commit {
		return doc.Path == strings.TrimSuffix(plan, ".plan.md")+".tests.json" || doc.Path == CataloguePath
	}
	return ready && doc.Kind == "inline" && doc.Path == "steerline-testing.md" && doc.Commit == "" && doc.Content != nil && *doc.Content == TestingInstructions
}
