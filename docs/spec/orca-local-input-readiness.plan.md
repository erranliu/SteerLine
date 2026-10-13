# Plan：Orca 本机输入与就绪

[Spec](orca-local-input-readiness.spec.md) · [专属验收](../acceptance/orca-local-input-readiness.md)。

1. 新生成独立包，脱敏比较本机管道/配对查询；核对 CLI 实现的本机 1 秒预检与配对路由，验证连接到同一本机 runtime。
2. 本机普通 CLI 使用已有配对认证，先校验本机 bootstrap 身份；修正等待预算、一次有界复查、失败诊断与明确拒绝的持久化。保持投递未知禁止重发，不新增旧路径兼容。
3. 补必要受控用例，运行下表；生产只读探针使用当前实现，失败如实记录。真实验收另建包含最终修改的新 live 包，运行任务 1，核验唯一专属 base/head/SHA 后合并，确认交付并清理原终端与服务。任务 2 不启动。
4. 更新专属验收与产品说明，整理提交 PR。

## 测试计划

| 场景 / 级别 | 自动项 | 环境、命令、时点 | 条件 / 证据 |
| --- | --- | --- | --- |
| OLI-INPUT / 定向单元 | OLI-A01 | 修改后隔离夹具：`go run ./cmd/test-feedback -- ./internal/execution ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -run '^(TestWorkerInputRejectedReceipt|TestOrcaInput.*|TestTerminalUnknownSubmissionOnlyObservesOriginal|TestOrcaNativeState(Stream|Identity))$' -count=1 -timeout=2m` | 仅一次就绪复查、交互阻塞无输入、预算正确、拒绝不变未知或重发、配对认证与身份不放宽；runner summary 登记专属验收 |
| OLI-NATIVE / 原生只读 | OLI-A02 | 新包：`scripts/test-orca-state-transitions.ps1 -PackageRoot <包> -OrcaExecutable <固定CLI>`；受控验证后 | 两次普通输入各一次，started/working/结束/完整正文、unknown outcome 保留，清理成功；包 evidence/native-production-lifecycle.json |
| OLI-LIVE / 真实交付 | OLI-A03 | `scripts/new-debug-release.ps1 -Orca -OrcaExecutable <固定CLI>`；注册包仓库，包 debug-helper debug run/control/snapshot；任务 1 到 await_merge 后 `gh pr ready` 与 `gh pr merge --merge --match-head-commit <SHA>` | 实现/报告/发布各一次，同一原生终端，专属 PR 精确核验并合并，任务 delivered；包 evidence 保存原生回执、快照、PR JSON 与清理，登记专属验收 |

不跑 full/L2/L3 全套；未改任务模板、远程 Git 同步或完成协议。OLI-A01 另跑普通终端原回归：`go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -run '^(TestAgentLaunch.*|TestTerminal.*|TestOrcaInput.*|TestOrcaNativeState(Stream|Identity|Gate|Lifecycle|Binding))$' -count=1 -timeout=2m`。受控阶段和共用 Codex 接线分为两个定向命令：`go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageOrcaNativeStateGate$' -count=1 -timeout=5m` 与 `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageLocalRoundTrip$' -count=1 -timeout=5m`，避免多个场景共用一个五分钟包预算。时点为修改完成后，条件是现有回合与阶段行为不退化，summary 登记专属验收。无人工验收项。
