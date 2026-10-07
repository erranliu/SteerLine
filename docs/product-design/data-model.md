# 任务、执行与交付数据模型

当前第一阶段仅运行和验收本地 Codex/本机 Orca。保留的远端身份/协议定义不表示当前支持，不构成一期前置；本文 repository 的 `remote` 指 Git 托管来源，仍用于本机 PR 交付。

SteerLine 尚未发布。当前生产配置必须显式设置 `"task_authority":"native"` 和 `"workflow_protocol":"worker-flow-v1"`。旧 `minimal-v1` 配置及 `workflow_definition`、`project_steps`、`workspace_dirty_ignore` 不属于生产协议。配置加载与正式服务入口在启动外部 worker 前拒绝旧协议；不导入、迁移、双写或覆盖旧现场。

## 权威与身份

SQLite 是 Task、固定 TaskContract、依赖、WorkflowRun、Execution、Attempt、Decision、Command、Operation、会话事件与 TaskDelivery 的唯一业务权威。格式版本见下表；旧格式只通过元数据检查识别并拒绝，不升级或开放业务读取，拒绝时保留原库与可能仍活跃的外部绑定。Runtime 是同一数据库快照重建的只读投影，不参与业务写回。

服务与会话检查点也保存在 SQLite；检查点不复制 Task、WorkflowRun、Execution、Attempt、Decision 或 Operation 行。连接观察只读检查点，恢复和命令在同一只读事务中把检查点与领域表组合。当前配置不使用 `runtime_file`，服务不导出 `runtime.json`。

TaskContract 固定目标仓库、remote、base 分支以及 Spec、Plan 和其他引用的 commit/SHA-256。领取前再次核验 Task 版本、依赖就绪、项目 authority、目标仓库和前置合并已进入当前 base。领取事务预留一个 Task、WorkflowRun、Execution 与固定 `worker` attempt；不生成 implement／verify／review／publish_pr 链。

| WorkflowRun 主阶段 | 条件 |
| --- | --- |
| `queued` | 领取已落盘，准备工作区或首次会话 |
| `executing` | 原 worker 正处理任务或反馈轮次 |
| `waiting_decision` | Codex 原轮次正在等待工具问题的回答，或轮次结束后需要反馈／外部结果无法自动核实 |
| `waiting_pr_merge` | 有已核实的 PR，worker 当前轮次已停下 |
| `completed` | 真实合并已核验，TaskDelivery 和 Task closed 已同事务提交 |

Task 的 `discarded` 是独立终态，不伪装成 Decision 或完成。连接、进程、回执不明和清理待办保存在 Operation、Execution 与事件中，不增设第二套可写主阶段。

Execution 绑定后端目标、工作区、分支、base、当前／待开始／已停下 turn ID、独立的 Command ID 及 turn 序号，还保存当前 Attempt 指针、服务实例归属、恢复门槛、版本和核验状态。local Codex 的附加身份为原生 thread ID 和 app-server 进程代次，协议为 `local_codex_app_server_v1`。本机 Orca 的附加身份为原生 agent terminal/incarnation、coordinator、Run 和每轮 Task/Dispatch，协议为 `orca_terminal_v2`；Windows Orca Codex 使用冻结权限的受管命令启动。普通反馈只推进 turn 和消息身份；确认旧 worker 终止后的人工继续可在同一 Execution 中建立下一 Attempt，旧 Attempt 保留原进程和会话身份。远端身份/反馈定义仅作保留协议资料，第一阶段不派发或验收该流程。

## 独立状态与资源位置

| 状态轴 | 记录与含义 |
| --- | --- |
| Task/DAG | open/closed/discarded、领取和依赖资格；“就绪”由依赖和当前约束决定 |
| 执行 | 原 Execution/Attempt、worker 会话、轮次、进程和结果不确定事实 |
| 交付 | WorkflowRun 主阶段与经真实 PR 核验的唯一 TaskDelivery |
| 反馈 | 原 Decision、first-wins 答案、Command/Operation 和平台接受回执；不要求“已应用”证明 |
| 派发 | 手动/自动模式、暂停与权威版本；不替代执行存活或交付状态 |

进程存活、消息接受、轮次结束和 Task 完成分别判断。服务实例更换不自动创建新的 worker；同一 Execution 的人工恢复可以增加新 Attempt，旧身份和回执保持可查。

STEERLINE_HOME 的默认值和路径解析由 [app/config.go](../../internal/app/config.go) 定义。省略 run_dir 时按 Git common-dir 派生 state/<repoKey>；显式 run_dir 经配置和宿主校验，必须位于受管仓库之外。服务同时锁定仓库 common-dir 和运行目录，同仓库不同 home 或 linked worktree 不能产生并行写者。工作区细节见[工作区准备](worktree-preparation.md)。

## 当前格式来源

此表是文档中的统一格式定位，以链接源码为准；其他设计引用本节，不重复维护“当前 schema”数字。

| 格式 | 当前值 | 源码 |
| --- | --- | --- |
| 工作流协议 | worker-flow-v1 | [model/state.go](../../internal/execution/model/state.go) |
| SQLite schema | 14 | [sqlite/reader.go](../../internal/storage/sqlite/reader.go) |
| 服务 checkpoint / runtime | 18 | [ports/checkpoint.go](../../internal/storage/ports/checkpoint.go) |
| FixedExecutionProfile | 6 | [execution_profiles.go](../../internal/execution/execution_profiles.go) |
| ConversationEvent | 2 | [model/conversation.go](../../internal/execution/model/conversation.go) |
| 全局执行配置 | 2 | [app/config.go](../../internal/app/config.go) |

身份版本、配置格式和某次验收的工具版本不是同一事实。历史报告保留其原版本，不能拿历史格式作为当前入口要求。

## 工作区与消息

初始工作区在外部创建前保存固定意图。local 路径使用固定 Git worktree 和原生 Codex session；本机 Orca 路径由 Orca 创建 worktree 和 agent terminal，再以新 Task/Dispatch 延续同一 terminal。重启核对原身份，不能按同名标题或路径推断替代会话。工作开始后，中心不会因反馈、dirty 文件、项目检查结果或 PR 出现而 reset、清理或改写 worker 的提交。

每次反馈有固定 Command ID 和 Operation。答案先在 SQLite 保存。local 运行中问题用仍有效的 app-server 工具请求 ID 回复，结束后反馈在原 thread 调用 `turn/start`；本机 Orca 运行中问题按原 message ID `reply`，活动补充向原 Dispatch `send`，结算后反馈在原 terminal 上创建新 Task/Dispatch。服务重开先核验旧执行和回执，旧的未送达输入不自动投递；用户须在统一 Decision 入口重新确认。已终止的 local worker 经核实后可用新受管进程和新 Attempt 恢复原 thread，明确选择后也可开新会话。平台接受回执不等于 agent 已处理。local 通知写入每轮 `appserver.jsonl`，完整正文由原生会话历史补读；Orca 输出按 worker-read 的 source、cursor 和 clipping 展示。回执不明时不更换 ID 重发。详见 [决策与反馈](decisions.md)和[停服恢复 Spec](../spec/worker-service-recovery.spec.md)。

人工模式允许不同 Task 分别领取；自动模式仍一次占用一个活动执行。暂停派发只限制新领取，不中断当前 worker、待决策反馈或 PR 核验。

## PR 与交付

worker 或用户提供的 URL 只是线索。中心经配置的 provider 查询真实 PR，并核对 URL、目标仓库、base、head 仓库和当前 Execution 的工作分支。查询失败保留线索并重试；关闭未合并返回待决策，仍可继续原会话。后续向同一 PR 推送不要求重跑中心 verify/review。

`CompleteNativeTask` 必须核实 provider 的 merged 状态、合并时间与有效 merge commit，然后在一笔 SQLite 事务中重核 Task 版本、选定 WorkflowRun/Execution、PR 归属及整任务发布绑定。事务唯一写入 TaskDelivery、关闭 Task、完成 WorkflowRun 并释放 DAG 后继。重复轮询和迟到事件重用同一交付，不回退终态。错误 PR、普通 closed、worker 自述完成或用户文字不能替代真实合并。

`task discard` 保留原有未合并核验与外部清理事实；状态不明时不静默关闭可能仍在运行的资源。已丢弃任务的 PR 若后来真实合并，只记录交付冲突，不生成正常 TaskDelivery。

## 控制入口

`steerline task list/show/graph/export` 只读中心权威；validate/create/discard/add-predecessors 通过固定版本与请求 ID 操作。控制台或 CLI 的 `answer_decision` 共享 CommandService 提交路径。正式服务入口只运行 `worker-flow-v1`；旧五步的 retry、mark-stopped、task reopen 和项目步骤验证器没有当前 CLI 入口。旧状态保留供人工核实，当前入口不读取或迁移旧业务数据。


## Windows Orca Codex 环境事实

Execution 的 `environment` 保存领取时的 Profile digest、准备 turn/nonce、开始及截止时间；资源绑定后追加原 runtime、已确认 terminal/incarnation、允许写入根目录和启动命令。Profile digest 包含受管 approval policy 与权限契约版本。准备轮次标为 `readiness`，业务轮次标为 `business`；准备不改变业务 CurrentTurn/LastStoppedTurn 或计数，同会话只有一个活动或结果未知的 Dispatch。

准备收据、类型化工具调用/结果证据和原生结算分别持久化，整批 Delivery 先落盘再 ack。宿主 blocker 保存 reason/confidence/source、原身份、时间及绑定目标，通过已有 Decision 与 Operation 提供中心动作；普通文字与原 worker question 分别保存。重复检查共享 blocker 的未完成 Operation；未知投递先读取原 Dispatch 历史。过期文字及问题保留历史，不改投新轮次。

准备超时和首次业务启动观察超时保持原外部事实。晚到准备成功只补历史，需显式重新检查后才能接纳；正常业务执行没有额外总时限。等待 PR 合并的终端诊断不回退主阶段，已提交交付不被迟到观察重开。当前格式拒绝缺少冻结准备/资源事实的受管 Execution，不迁移旧记录。实际验收状态见[环境验收记录](../validation/worker-environment-readiness/README.md)。

## 本地 Windows sandbox 配置冻结

冻结的 FixedExecutionProfile 只保存通用策略、实现 key/version 和有界的实现配置封套；Codex codec 拥有 `windows_sandbox`（`elevated` / `unelevated`）并将有效配置纳入 Profile digest。Windows local 受限方案的有效默认值为 `elevated`，领取时随封套冻结。PolicyResources 保存通用 workspace/权限根及 Codex 资源封套；ExecutionRecord 的原生 thread、endpoint 与 process generation 位于 Codex binding envelope，Orca terminal、Dispatch、coordinator 和 cursor 位于 Orca execution-state envelope。恢复读取原冻结 Profile 与原绑定，不使用当前全局配置改写已开始的 Execution。格式版本统一见[当前格式来源](#当前格式来源)；格式变化拒绝旧数据，不迁移或回填。

Decision 的实现问题路由也复用同一 64 KiB 封套，以 answer purpose 固定原 WorkflowRun、Execution、Attempt、generation、turn 和 question；回答路由不能冒充配置或资源封套。
