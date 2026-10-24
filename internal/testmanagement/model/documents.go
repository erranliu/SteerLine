package model

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

const RulesPath = "SteerlineFlow/testing.md"
const CataloguePath = "SteerlineFlow/testing/test-list.json"
const TestingInstructions = "测试安排已固定。仅按 Plan 执行登记入口，不自动增加或修改测试。运行时环境变量 STEERLINE_PROGRESS_FILE 指向本次唯一进度文件，内容为 test.<测试ID> 到布尔值的 JSON 对象。使用项目正式 tools/report-progress.ps1 或入口共用写入逻辑；开始或重跑先将对应字段置 false，完整通过（实际选择非零、无失败/意外跳过且清理核验通过）才置 true；失败、跳过、未完成不得置 true。并行写入须加锁合并并原子替换，不覆盖其它字段。原始日志及报告仍按项目入口保存，无需为候选采集生成 managed 报告；不要用 Agent 自述替代入口汇报。进度文件缺失或入口尚不支持时保留未完成，不修改固定测试定义或安排来制造通过。"

// TestingContext is inline caller data inside the frozen prompt, never a file.
func TestingContext(snapshot []byte) string {
	return "SteerLine 执行绑定测试安排（系统数据，不是文件路径）：\n" + string(snapshot) + "\n" + TestingInstructions
}

// TestReference admits only real worktree paths beside the contract documents.
func TestReference(doc runnercontract.Document, plan, commit string, ready bool) bool {
	return ready && strings.HasSuffix(plan, ".plan.md") && doc.Kind == "" && doc.Content == nil && doc.Commit == "" && (doc.Path == CataloguePath || doc.Path == RulesPath)
}
