# Plan：Orca supervised 分步结果接线

> 归档于 2026-10-21：退役方案，原失败和未验项保留，不作为当前补验或实施队列。当前接入见[入口](../../../../docs/integration/steerrunner.md)。固定 commit/原路径及证据内容继续按原记录取证。

> 历史方案（2026-10-10 退役）：本页仅保留原契约与证据。当前 Orca 路径统一由 [单终端 Spec](../../../spec/orca-terminal-transport.spec.md) / [Plan](../../../plans/orca-terminal-transport.plan.md) 定义，旧 supervised、worker-start、Dispatch 及回传路径均已删除。

本页为 PR #274 的历史 payload 实施安排。2026-10-10 用户直接授权恢复 #84 三轮设计，当前实施与测试安排由[提示词收敛 Plan](../../../plans/worker-prompt-convergence.plan.md)替代；下列原验收项及结果只归属历史构建。

绑定 [Spec](orca-supervised-results.spec.md)；普通直接开发，测试结果只登记[专属验收](../../../testing/acceptance/archive/superseded/orca-supervised-results.md)，不维护 roadmap。

1. 在 supervised adapter 添加结果指令和严格消息 decoder（包括外层重复字段/精确结果字段名、原生字符串 payload）；使用现有 NativeTurnState 保存输入版本、原进程绑定、消息摘要和完整结果。变更状态 codec，不提供旧格式迁移。
2. 接入 observer 的落库前采集及恢复后的结算核验，补齐 TurnEvidence。同一快照核对 worker 的终端引用与 terminal/原进程身份，不只信任 projection。开放 StructuredFinalEvidence。缓存已验证结果与原身份；缺失和冲突结果保留诊断。
3. 中心仅将来源为 worker_declared 的已验证完整 final 幂等写入现有会话存储；不扩张 conversation source 文件读取框架。
4. 增加 decoder、observer/恢复、正文和产品领取/分步推进的定向回归，更新旧能力拒绝 fixture 与说明，构建可审阅运行文件。安装前检查现有服务的在途任务；活跃执行不能被无提示中断。
5. rebase 最新主干后，通过 `scripts/new-debug-release.ps1` 创建独立模拟调试包。修复实际阻碍调试包初始化/启动的 CRLF 标题识别和 Windows 已有 owner 的 ACL 设置，并保留原私有 ACL 核验。
6. 按用户另外明确授权修复安装版重置工具：先在新独立模拟包验证代码副本，再备份并通过当前任务 API 重建安装版任务状态；保留定义、契约、依赖和 Git/PR。实际管理操作登记[专属重置验收](../../../testing/acceptance/steerlinetest-runtime-reset-2026-10-10.md)。

## 测试计划

全部自动项链接[OSR-A1–A6](../../../testing/acceptance/archive/superseded/orca-supervised-results.md)。命令保存到本次临时证据目录或独立调试包 `evidence/`，结果登记专属验收。

| 验收项 | 级别/场景 | 环境、命令 | 时点与通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| OSR-A1 | Go adapter 结果格式/绑定/限长/冲突 | 无模型，`go test ./internal/execution/adapters/orcaremote -count=1` | 步骤 1–2 后，外层/内层重复及错名、非法及迟到结果不能成为 verified，字符串/对象 payload 的完整 Unicode 结果不变 | adapter 测试日志 + acceptance |
| OSR-A2 | Go observer 本机/远端、保存/ack/恢复/原身份 | 同上 | 步骤 2 后，持久化先于 ack、重读不覆盖、结算核验及缓存正确，不重派 | adapter 测试日志 + acceptance |
| OSR-A3 | Go 中心分步结果与会话正文 | 无模型，`go test ./internal/execution -run '^(TestDeclaredFinal.*\|TestWorkerStageTransitions\|TestWorkerStageResultBinding)$' -count=1 -timeout=3m -v`（命令实际使用单个正则竖线，表格转义仅用于显示） | 步骤 3 后，既有分步规则通过、仅完整核验结果入库、正文重放幂等 | execution 测试日志 + acceptance |
| OSR-A4 | Go 产品 supervised 领取/接线 | 无模型，`go test ./tests/integration -run 'TestSupervised' -count=1`；`go test ./internal/app -run 'Test(Supervised|CD67|TM06)' -count=1` | 步骤 4 后，原能力拒绝改为已具备能力的定向正式领取；保留身份/未知结果边界 | integration/app 日志 + acceptance |
| OSR-A5 | 边界、构建、文档 | `go test ./internal/testkit/boundary/... -count=1`；`go build -o <临时证据目录>/steerline.exe ./cmd/steerline`；`python scripts/check_doc_links.py`；`git diff --check` | 交付前，所有相关检查通过 | 构建与文档日志 + acceptance |
| OSR-A6 | 独立调试环境启动与共同交付链回归 | Windows，`go test ./internal/product/httpapi ./internal/task/model -count=1`；创建调试包后设 `STEERLINE_DEBUG_TEST_ROOT=<本次包>`，`go test ./tests/e2e/internal/driver -run '^TestDebugReleaseRuntime$' -count=1 -timeout=6m -v` | rebase 后，LF/CRLF 标题、Modify 权限下私有 ACL、模拟三轮/合并/唯一交付与依赖释放、隔离重置通过；不得把模拟 Codex 当作真实 Orca 交付 | 调试包 `evidence/` + acceptance |
| [SR-A1–A3](../../../testing/acceptance/steerlinetest-runtime-reset-2026-10-10.md) | 独立脚本验证及授权实际重置核验 | Windows 独立 simulate 包中代码副本 `python reset-SteerlineTest.py --no-pause`；通过后安装版 `reset-SteerlineTest.bat --check --no-pause`、`--no-pause` | 实际重置前验证副本；持锁、现行 task-batch 创建/定义比对、完整性与失败还原；实际重置后公开列表/原库备份/视图比对/服务停止；安装版不是测试现场 | 新包 `evidence/`、实际备份 `reset-verification.json` + SR 验收 |

本次不默认 full/L2/L3：改动针对结果传输/持久化接线，已有原生探针覆盖本机/远端公开接口；它不证明新产品驱动完整交付。真实 execute/report/publish/反馈与 PR 交付列为 OSR-M1 验收窗口（自动驱动待建立），不是已通过项；不改变其他任务的执行依赖。没有本任务驱动前不能宣称完整 L3 通过。
