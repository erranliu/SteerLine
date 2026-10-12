# Plan：旧 Orca 编排身份残留清理

按 [Spec](retired-orca-identity-cleanup.spec.md) 实施，仅整理当前普通开发范围。

1. 核对两项内置 registration、claim 构造和全部源码消费点，删除旧 target/process 字段、旧比较/准备条件及 host 死函数；保持当前资源绑定和 actor/事务。
2. 用当前终端身份往返与逐字段旧格式拒绝场景替换旧 supervised 保留测试，已有清理回执测试改用 WorktreeID，保留业务断言。
3. 依据已提交的真实验证范围更新当前文档；execution-map/既有一期草稿只同步汇总，不增删执行成员或改变历史证据。
4. 执行最小测试，向 [专属验收](../acceptance/retired-orca-identity-cleanup.md) 登记来源、命令、结果和证据。

## 测试计划

| 验收项 | 级别/场景、命令 | 环境/时点 | 通过条件与证据登记 |
| --- | --- | --- | --- |
| [RC-A1](../acceptance/retired-orca-identity-cleanup.md) | 定向 Go：`go run ./cmd/test-feedback -- ./internal/execution -run '^(TestExecutionIdentity.*|TestSimplifiedCleanupRetainsFrozenDeliveryEvidence|TestWorkerStageResultBinding|TestWorkerEnvironment.*|TestEnvironmentRecheck.*|TestSimplifiedEnvironment.*|TestManagedFrozenResource.*)$' -count=1` | Windows，现有隔离夹具；删除及测试调整后 | 新场景非零执行，当前身份保持、9 个旧字段拒绝、清理不改写结算事实，已有准备规则保持；默认临时 summary 路径登记验收 |
| [RC-A2](../acceptance/retired-orca-identity-cleanup.md) | 定向 Go：`go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStage(LocalRoundTrip|OrcaTerminalRoundTrip|OrcaEnvironmentRoundTrip)$' -count=1` | 现有隔离 SQLite/Git/受控进程夹具；代码完成后 | 三个场景均执行且通过，同会话三阶段、当前 target/process、重启不重投与原结果保持；默认临时 summary 登记验收 |
| RC-A1（必要存储回归） | `go run ./cmd/test-feedback -- ./internal/storage/sqlite -run '^TestSQLiteRejectsParallelExecutionsInOneWorkflow$' -count=1` | 现有隔离 SQLite 夹具；删除 fixture 的旧 Dispatch 字段后 | 同一 Workflow 仍拒绝第二个 Execution，已有快照断言保持；默认临时 summary 登记验收 |
| RC-A1、RC-A2（必要构建回归） | `go test ./... -run '^$'`；`go test -tags=e2e ./tests/e2e/... -run '^$'`；`go vet ./internal/execution/... ./internal/app/...` | Windows；交付前 | 全消费者及带 tag 场景编译，定向 vet 通过；临时日志登记验收，仅编译不算场景通过 |
| [RC-A3](../acceptance/retired-orca-identity-cleanup.md) | `python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check`；逐项核对单终端验证来源与汇总 | 文档修改后 | 本地导航、身份、格式通过，当前与历史分开且未扩大通过范围；临时日志与人工源码/文档核对摘要登记验收 |

不运行 Full/L2/L3/live/native：不改原生 transport、投递、停止、PR 或恢复授权；受控两类目标三阶段验证和严格格式/清理断言覆盖本次变化。新增场景只覆盖删除字段造成的契约变化；无需新增测试环境。无独立人工验收项，本次文档维护不结算一期原生签收。
