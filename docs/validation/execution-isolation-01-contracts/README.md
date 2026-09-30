# 公共执行契约归包验证

日期：2026-09-30。任务 #54；对应 [Spec](../../spec/execution-isolation-01-contracts.spec.md) 与 [Plan](../../spec/execution-isolation-01-contracts.plan.md)。基线 `c2c9f95`（包含 PR161 与新合入的 PR160）。本次只交付第一轮公共契约。

## 类型所有者与交接

| 新所有者 | 实际消费者与后继交接 |
| --- | --- |
| `execution/backend.go` | Backend/Protocol、ExecutionRef、ProcessIdentity、Probe/Backend/Stopper 和 StopHandle；中心、平台过程、会话与控制台直接使用。Stopper 接收原资源句柄，中心先保存停止意图 |
| `execution/operation.go` | 操作身份/摘要/状态、停止回执、LaunchCommand；已有绑定/操作附件事务、停止接线与 Codex app-server 命令直接使用，保留 JSON 格式 |
| `execution/capability.go`、`errors.go` | 能力状态/报告和两类错误链；已有能力证据存取、配置预检、适配调用直接使用；verified 不由静态声明推定 |
| `execution/workspace.go` | ExecutionTarget、WorkspaceIntent、WorkspaceInspection；现有工作区准备/观察直接使用；#55/#56 接收固定目标、资源意图和核验结果 |
| `execution/session.go` | SessionContext、Question/DecisionOption、TurnInput、Answer；中心转换固定记录，Codex 输入/回答/问题解析与 Orca 问题事实直接使用；平台回答路由仍由现有实现核验 |
| `execution/event.go` | 观察/输出批次、EventFact、ToolReplyFact、TurnEvidence 与唯一 PRClues；Codex 与 Orca 共用 PR helper，公共事件保留来源、游标和原始日志定位 |

中心仍拥有业务记录、OperationRecord、TaskDelivery、SQLite 和原子附件事务。`worker_evidence.go` 保留记录到 SessionContext 的转换与带原生回答路由的中心桥；`workerTurnEvidence` 只包装公共观察值和中心的发布/问题判定。Codex app-server/AskEnvelope 与 Orca Message/Dispatch/settlement 解析留在各自唯一实现，#57/#58 再迁移生命周期接线。未迁移的 Git、PR/provider 实现分别交给 #55/#56；当前模式选择和组装留待 #61。没有新增旧类型别名、转发 API、双实现或回退路径。无人消费且携带 Run/Worker 的 StartReceipt 已删除。

## 实际验证

- `go test ./internal/steerline/execution ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/codexapp -count=1`：通过。
- `go list -deps ./internal/steerline/execution` 及非标准库依赖过滤：通过；[依赖记录](dependencies.txt)表明项目内传递依赖只有公共包本身，源码不执行文件、进程、网络或数据库 I/O。
- 项目 `go run ./cmd/test-feedback -quick` 加定向中心/控制台回归：通过；[最终摘要](quick-summary.json)保存真实分组、命令、测试数量（含子测试）与结果，没有零命中分组。
- `GOOS=linux GOARCH=amd64 go build ./cmd/steerline`：交叉编译通过，产物写入临时目录；未在 Linux 运行或做平台认证。
- 223 个本地文档链接与 `git diff --check`：通过。中心、控制台、cmd/steerline 与三个适配包的测试编译通过。

原六个 Plan 回归名与包路径全部保留。整合 PR160 时保留新增的 Receiver/Action、环境记录和恢复检查，公共选项新增相应 JSON 断言；追加两项已有宿主等待/冻结资源回归，并单独运行 `TestWorkerEnvironmentER1ProbeAndSettlementGate/success`，通过。另复用 `TestWorkerBindingRollsBackWithItsCheckpoint`、`TestExecutionStopIntentAndUnknownReceiptUseOriginalBinding`、`TestSupervisedTurnEvidenceRetainsQuestionAndEarlierPR` 及一个已有控制台答案编辑 fixture。新增公共包三项纯测试只覆盖搬迁相关的 JSON 表示、错误链与 PRClues；原 Codex/Orca fixture 增补公共问题、来源/日志定位和 input_accepted 不等于 turn_started 的断言。原生解析 fixture 留在适配包，持久记录和事务回归留在中心，没有重复迁移整套矩阵。

EC1 对应依赖与唯一所有者检查；EC2 对应固定绑定、能力、记录损坏、错误链及原子停止；EC3 对应两平台原身份/问题/终态 fixture 和监督路径 journal 结算。绑定 schema、操作表示、权限配置、消息持久化后确认顺序和人工恢复门槛保持。未运行全量测试、真实 agent 或真机认证；没有部署或修改中心任务数据库。

## 实现质量审计

完成[代码审计与两处直接修复](audit.md)，追加的最小验证见[审计测试摘要](audit-summary.json)。原实现的验证摘要保留，新增结果单独记录。

## 主干查询边界集成

整合 PR163 后，`contracts` 定义持久记录与查询 DTO，并直接引用 `execution` 的身份、目标、工作区意图、问题和选项。移除与公共执行类型重叠的声明及中心旧别名，保留主干 Query/SQLite 读取能力接线、业务实体与原子写入所有者。Codex/Orca 的持久回答路由沿用 `contracts` 定义，其 Questions 使用唯一公共类型；本轮两个审计修复保留。
