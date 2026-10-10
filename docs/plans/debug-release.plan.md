# 独立调试环境实施 Plan

统一由 `scripts/new-debug-release.ps1` 复制 `debug-template/`，构建当前 exe/helper，初始化独立 SteerlineTest Git 和任务数据库。执行配置在复制源维护，默认 live；AI 显式选择 simulate，首次使用后冻结方案。安装环境不参与调试。生成不调用模型或发布 PR。

## 实施步骤

1. 新建复制源：项目、执行配置、任务定义、Spec/Plan、启动和无备份重置入口。
2. 生成脚本构建到新目录，路径预检，记录源码/模板/产物身份，正式 task validate/create 初始化数据库。
3. 包内运行封装实现首次方案选择、隔离认证、本地模拟服务、真实 SteerlineTest base 分支发布。
4. helper 实现任务状态重置和锁检查，无备份、不回退 Git 或 PR；增加独立环境驱动和定向测试。
5. 更新 AGENTS、调试文档及测试入口，已有自动测试生成现场接到统一入口。

## 测试计划

验收归属：[独立调试环境](../acceptance/debug-environment.md)。证据存本次 debug 包 `evidence/` 和 runner 输出目录，结果登记在该验收文档。

| 验收项 | 级别/场景 | 环境/命令 | 时点、通过条件 |
| --- | --- | --- | --- |
| DBG-01、DBG-04 | 定向脚本 `debug.release-generation` | Windows controlled；`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-debug-release.ps1` | 生成完成后，两包独立、污染环境不影响、拒绝链接/冲突，无安装写入 |
| DBG-02、DBG-03 | 定向 Go `debug.release-runtime` | Windows controlled；`go test ./tests/e2e/internal/driver -run DebugRelease -count=1` | runtime/reset 完成后，模拟三轮真实 Git/PR 事实，锁阻止重置，停服后清除状态且保留任务/依赖/Git，无备份 |
| DBG-05 | 显式真实 PR `debug.release-live` | Windows live；`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-debug-release.ps1 -Live` | controlled 通过后，生成新包、发布独立 SteerlineTest base 和 PR，核对并清理精确分支；不默认调用模型 |
| 回归 | L1/受影响 helper | `go run ./cmd/test-feedback -level=L1`；`go test ./tests/e2e/internal/controlled -count=1` | 交付前，固定最小入口及模拟 helper 回归通过 |

新增定向脚本/helper 驱动随实施提供。不跑 full、完整 L2 或其他 L3：本次范围是环境生成与入口，已有产品交付行为通过定向模拟链路检查。真实 Codex 人工控制台体验单列非阻塞人工项，不能用真实 PR 脚本冒充模型交付验收。
