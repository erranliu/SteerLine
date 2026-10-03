# #71 产品客户端交付与验证

执行日期：2026-10-03–04（Asia/Shanghai）。基线为 `184e1e06d217b243e5655f4f9b7c233e4c80f579`（#68 / PR191）；提交前再次核对远端 main 仍为该基线。工作区 `C:\Users\lmas1\.codex\worktrees\714a\SteerLine`，分支 `codex/71-product-client`，模型 `gpt-6.1-sol / high`。

## 实际消费者与所有者

| 入口 | 实际调用与保留语义 |
| --- | --- |
| `task show/list/graph/export` | 产品 TaskClient → 注入 native Reader → 原 QueryService / taskview；原 cursor、采样版本、导出输出和禁止覆盖 state 文件的规则保留 |
| `task validate/create/discard/add-predecessors` | 类型化产品输入 → 注入资源工厂 → 原 CommandService；仍由中心校验资格、版本、幂等键并提交，初始化与仓库锁沿原顺序 |
| `tasks` | 唯一客户端调用原 ReadTaskSnapshot，不另建投影 |
| `control/stop/stop-service` | 类型化 control 输入 → 原文件 control/StopService；首次发送前固定 ID，查询和等待只核对原请求；同 ID 同内容返回原请求，异内容、实例或版本冲突拒绝 |
| `answer` | caller 固定 CommandID 后调用原 SubmitDecisionAnswer / SubmitAnswerDecision；`answer -request-id` 只查询原命令，accepted 保持受理含义；未知结果不补发或换目标 |
| console 默认源 | `FileQuerySource/FileControlClient` 只映射到相同产品小接口；项目组合快照、正文/raw、能力、活动日志、候选输入检查由唯一 adapter 提供 |

`clientcontracts` 只依赖 stdlib 与 contracts/execution/task/taskview。连接、观察、control 请求/载荷、stop 结果和日志游标的唯一类型定义已从中心移入纯包；中心仅保留内部接线别名，产品接口直接使用纯包。共享 contracts 的既有字段未调整。

`steerlineclient` 接受 `clientcontracts.Factory`，没有 SQL 类型、数据库解释或 app import。`steerlineapp/client.go` 只构造原 reader/query/command/control/配置资源，不承担业务转换。读取与命令句柄在操作内关闭，关闭错误传播；Client.Close 幂等并拒绝后续操作，绝不停止服务/worker。离线读不建库、迁移、准备工作区或消费命令；缺库、格式、归属、未知记录与其他读失败有产品错误分类。回执需匹配不可变请求及当前项目，读取失败不会显示成成功空页。

## 删除与 #72 交接

删除 console 的日志 reader/normalizer I/O、默认源中的开库/query/control/decision 实现，以及 CLI 的直接开库、命令轮询和 profile 请求构造。移除 console 不再消费的 normalizer 字段/配置接口；候选输入检查移至 adapter。

#72 当场删除：`FileQuerySource`、`FileControlClient`、`productRef`、`withClient`、`candidateFreshness` 映射；console 旧 QuerySource/ControlClient/DecisionCommandClient、ProjectRef.Config/ConfigSource、model 中的默认桥构造和中心别名/配置视图。改为直接注入 Lifecycle/TaskClient/ProjectQueries/ConversationQueries/Controls/Decisions，保持现有导航/渲染语义。本次没有实施完整 TUI 迁移。

并行修改段：新增 `steerlineapp/client.go` 的客户端构造，`steerlineapp/console.go` 的客户端注入及 raw normalizer 选择，CLI 产品消费段；中心 `decision.go` 仅允许原 CLI answer 接收 caller 已分配的 CommandID。未修改 #69 写端口、Store/schema、业务提交接线或 contracts 原共享字段；quick 和 #67 守卫选择不变。

## 验证与非零命中

Windows / Go 1.26.4 / 本地 Git、SQLite、文件 control；无真实模型请求。原始证据：`C:\Users\lmas1\AppData\Local\Temp\steerline-cm71-evidence-58312818`。定向结果和 quick 摘要保存在 [test-results.json](test-results.json)，其中列出确切命中和日志 SHA-256。

| 实际命令 | 结果与证据 |
| --- | --- |
| `go test -json ./internal/steerlineclient -count=1` | 11 顶层 / 21 run-pass，0 fail/skip；`client.jsonl` |
| `go test -json ./cmd/steerline ./internal/steerlineapp -run 'Test(NativeTaskBatchCLI\|CLIExecutionProfile\|WaitServiceCommand\|CM71\|FileQuerySource)' -count=1` | 5 顶层 / 9 run-pass，0 fail/skip；`consumers.jsonl` |
| `go test -json ./internal/steerline/query ./internal/steerline/console -count=1` | 69 顶层 / 73 run-pass，0 fail/skip；`query-console.jsonl` |
| `go test -json ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1` | 1 顶层 / 10 run-pass，0 fail/skip；全标签、Windows/Linux 图通过；`boundary.jsonl` |
| `go run ./cmd/test-feedback -quick -out <证据目录>\quick` | 稳定后一次，15 包 / 144 run-pass，7 组全部通过，0 fail/skip；`quick/summary.json` |
| `git diff --check` / `git diff --cached --check` | 通过，无空白错误 |

CM71-2 的真实 CLI 子用例在正式 Service 构造的进程身份和 SQLite checkpoint 上，走真实文件请求/回执；未调度 worker。断言 fixed project/instance/control revision、未知回执保持 indeterminate、同 ID 异内容拒绝、关闭客户端后同服务仍 connected、原请求字节不变，以及自洽外项目请求/回执也拒绝。

迁移映射：原 `console/activity_source_test.go` 四个 `TestConsoleSource*` 同名迁到 `steerlineclient/activity_test.go`；原 console 的 snapshot reuse / Ready 501 项顺序用例同名迁到 `steerlineclient/snapshot_test.go`；原 `console/project_source_test.go` 同名迁到 app；原 CLI profile 用例同名迁到 app，原 wait-command 用例迁到 adapter，以纯 reader 注入检查原 ID、accepted、延迟、关闭、未知及失败。真实 Git 身份解析另由 app profile、项目快照和完整 CLI batch/control 断言覆盖，没有删除必需断言。

早期定向失败先诊断后修复：profile 测试比较了未解析身份的旧配置副本；新增真实 CLI control 夹具缺失 claimant，中心正确拒绝 identity-error。分别修正夹具身份解析与显式 claimant，保留原摘要/身份断言后通过。自查补上回执的项目归属校验和缓存复用前的当前配置连接校验。

未选 L2/L3/full：未改变执行、交付、恢复或原生权限映射；所选 query/control/CLI、现有 quick 与 #67 足以覆盖本次客户端边界。完整 Spec/Plan、实际 diff 与受影响生产调用路径已自查，无已知未解决问题；PR 不合并，等待协调聊天独立复审。

## 协调聊天独立复审与并行集成

2026-10-04 将 #69 PR194 的已合并主干 `e81fadaf59d0411af1594be7c5cd6a832c71c29f` 集成到本 PR（集成 commit `fc5c6cc1c7c69a5965b09aea11b63c0e6ce3c376`）。移动后的项目快照测试保留 #69 的 BatchTx 和测试本地损坏数据库 fixture，也保留 #71 的注入工厂及全部原断言。

集成验证发现新 CLI control 夹具仍调用已删除 DomainTx，导致编译失败；协调聊天在 `2f2cdc34801c8286f3dd435fad7a3d996515ec51` 改用既有 Metadata.SaveCheckpoint，未改身份/版本、原请求、未知回执、外项目拒绝或关闭断言。架构文档同步真实集成基线及当前存储/日志所有者，333 个本地链接均存在。

协调聊天亲自读取 Spec/Plan、实际 diff、产品契约、CLI/adapter/app/console 路径、资源关闭、错误/回执归属及测试迁移映射。修复后实际验证：

- `go test -json ./internal/steerlineclient -count=1`：11 顶层 / 21 run-pass。
- `go test -json ./cmd/steerline ./internal/steerlineapp -run 'Test(NativeTaskBatchCLI|CLIExecutionProfile|CM71|FileQuerySource)' -count=1`：5 顶层 / 9 run-pass。
- `go test -json ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1`：1 顶层 / 10 run-pass。
- 集成稳定后一次 `go run ./cmd/test-feedback -quick -out C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-core-decoupling-20261003-01a10252/cm71-final-quick`：7 组 complete/passed，非零命中，0 fail/skip。
- `git diff --check` 通过。上述验证后仅更新文档，生产与测试源码保持测试时版本。

原始失败、修复后 JSON、quick 报告及 PR 审核/合并门禁证据保存在本 run `C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-core-decoupling-20261003-01a10252`，历史执行聊天的验证记录仍保留。无验收例外；未新增 L2/L3/full。
