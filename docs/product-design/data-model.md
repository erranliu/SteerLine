# 任务、执行与交付数据模型

第一阶段包含本地 Codex、本机 Orca 与远端路径交付。本文 repository 的 `remote` 指 Git 托管来源。

SteerLine 尚未发布。当前生产配置必须显式设置 `"task_authority":"native"` 和 `"workflow_protocol":"worker-flow-v2"`。旧 `minimal-v1` 配置及 `workflow_definition`、`project_steps`、`workspace_dirty_ignore` 不属于生产协议。配置加载与正式服务入口在启动外部 worker 前拒绝旧协议；不导入、迁移、双写或覆盖旧现场。

## 权威与身份

SQLite 是 Task、固定 TaskContract、依赖、WorkflowRun、Execution、Attempt、Decision、Command、Operation、会话事件与 TaskDelivery 的唯一业务权威。格式版本见下表；旧格式只通过元数据检查识别并拒绝，不升级或开放业务读取，拒绝时保留原库与可能仍活跃的外部绑定。Runtime 是同一数据库快照重建的只读投影，不参与业务写回。

服务与会话检查点也保存在 SQLite；检查点不复制 Task、WorkflowRun、Execution、Attempt、Decision 或 Operation 行。连接观察只读检查点，恢复和命令在同一只读事务中把检查点与领域表组合。当前配置不使用 `runtime_file`，服务不导出 `runtime.json`。

工作区准备意图、轮次观察及当前决策绑定由 Execution 保存。回答事务校验 WorkflowRun 的持久控制版本；服务心跳和日志更新不改变该版本，不能把观察刷新当作业务反馈授权或新的请求身份。

TaskContract 固定目标仓库、remote、base 分支以及 Spec、Plan 和其他引用的 commit/SHA-256。领取前再次核验 Task 版本、依赖就绪、项目 authority、目标仓库和前置合并已进入当前 base。领取事务预留一个 Task、WorkflowRun、Execution 与固定 `worker` Attempt，并固定开发流程的 implement.execute 起点；模型步骤不新增 DAG Task。

WorkflowRun 在原记录中保存唯一 `status/flow_ref/step_id`，不并存旧 Phase；当前 `flow_ref=development/v3`，与 `worker-flow-v2` 协议版本分别固定。`stage_inputs` 保存实际输入、Command、Attempt、输入版本和报告源 Command/turn；`stage_results` 保存原生终态、规范 final 来源、输入绑定和 outcome，不保存独立 artifact 快照。输入/已接受结果不可替换，重复 first-wins。正常三个模型步骤复用固定 `worker` Attempt；该名称是原生执行角色，不是另一份可写业务位置。规则来源为[model/staged_flow.go](../../internal/execution/model/staged_flow.go)与[staged_flow.go](../../internal/execution/staged_flow.go)。

| status | 条件 |
| --- | --- |
| `queued` | 已保存指定步骤的冻结输入意图，尚未证实原生开始 |
| `running` | 原生开始已证实 |
| `waiting_input` | 原目标有明确问题/反馈等待答案 |
| `blocked` | 已知失败、缺能力或前置不满足 |
| `reconciling` | 原输入、结果或外部副作用未知；只读对账 |
| `waiting_external` | 本开发流程的 PR 已核验，等待真实合并 |
| `completed` / `abandoned` | 经唯一交付核验的完成 / Task 丢弃后的工作流终态 |

开发步骤为 `implement.execute → implement.report → publish → [publish.report] → await_merge → delivered`。实现自由 final 不能作为结构化结果，报告绑定原执行，发布有合法结果时直接消费；缺结果或格式错误仅补一次 publish.report，绑定原发布及冻结候选，见[发布补报契约](../spec/worker-publish-report.spec.md)。源失败不被报告成功覆盖；报告最多自动重试一次，发布不为补 JSON 重做副作用。报告沿用冻结权限及该后端实际工作区检查能力，Orca 的 revision 范围不证明未提交内容干净，也不保证拦截外部写入。无 artifact 快照的已提交报告恢复不能沿旧快照重试；必要时回原会话重新核实实现，仍保留原人工恢复门槛。

Task 的 `discarded` 是独立终态，不伪装成 Decision 或完成。连接、进程、回执不明和清理待办保存在 Operation、Execution 与事件中，不增设第二套可写主阶段。

Execution 绑定后端目标、工作区、分支、base、当前／待开始／已停下 turn ID、独立的 Command ID 及 turn 序号，还保存当前 Attempt 指针、服务实例归属、恢复门槛、版本和核验状态。local Codex 的附加身份为原生 thread ID 和 app-server 进程代次，协议为 `local_codex_app_server_v1`。Orca 的本机 / 远端附加身份均为原 runtime、agent terminal/incarnation、worktree、pane 和 provider session，协议为 `orca_terminal_v3`；每轮保存 Command、原输入回执、发送前主代理基线、结束证据及不可替换的完整普通回复，启动与权限由目标主机的 Orca 管理。不存在 Orca Task/Dispatch/coordinator 绑定或模型回传标记。普通反馈只推进 turn 和消息身份；确认旧 worker 终止后的人工继续可在同一 Execution 中建立下一 Attempt，旧 Attempt 保留原进程和会话身份。

## 人工候选实验

同一 Task 默认一个候选，人可逐个增加不同冻结配置的候选。每个候选具有独立 WorkflowRun、Execution、会话、工作区、分支、PR 和反馈；开始时观察参与数已达五个就拒绝，接受竞态偶尔超出，不建设硬名额、预留、队列、自动补位或评分融合。

产品不提供单候选放弃操作，控制台、CLI、API 和服务端均无此入口。Task 丢弃仍按任务级规则执行，原执行停止和资源结算按事实记录。任一已绑定候选 PR 经其冻结提供方核验已合并、仓库/base/head 分支身份与合并证据完整后即可原子提交唯一 TaskDelivery，将 Task 转为 closed 并释放后继；不要求其他候选的 PR 可查询或未合并，不等待已绑定 PR 的追加反馈结束。首个成功提交的交付记录成为获胜候选，不按合并时间排名或融合成果。提交时复核获胜候选自身绑定、任务/图版本以及 PR 不被重复绑定，其他候选阶段推进不阻塞交付。closed 后停止该任务全部候选的 PR 状态查询，重启后仍跳过；界面保留关闭时最后已知的 PR 事实。候选历史、资源与未结算执行保持准确归属。界面选择及草稿隔离见[执行控制台](execution-console.md)。

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
| 工作流协议 | worker-flow-v2 | [model/state.go](../../internal/execution/model/state.go) |
| 固定开发流程 | development/v3 | [model/staged_flow.go](../../internal/execution/model/staged_flow.go) |
| SQLite schema | 19 | [sqlite/reader.go](../../internal/storage/sqlite/reader.go) |
| 服务 checkpoint / runtime | 21 | [ports/checkpoint.go](../../internal/storage/ports/checkpoint.go) |
| FixedExecutionProfile | 9 | [execution_profiles.go](../../internal/execution/execution_profiles.go) |
| ConversationEvent | 3 | [model/conversation.go](../../internal/execution/model/conversation.go) |
| 全局执行配置 | 2 | [app/config.go](../../internal/app/config.go) |
| Codex profile codec | 2 | [codex/profile_config.go](../../internal/execution/adapters/codex/profile_config.go) |
| Codex resource codec | 2 | [codex/policy_resources.go](../../internal/execution/adapters/codex/policy_resources.go) |
| Orca profile / state codec | 2 / 5 | [orcatransport/profile.go](../../internal/execution/adapters/orcatransport/profile.go) |
| 内建执行目录 | builtin-execution-v3 | [app/directory.go](../../internal/app/directory.go) |

身份版本、配置格式和某次验收的工具版本不是同一事实。历史报告保留其原版本，不能拿历史格式作为当前入口要求。

## 工作区与消息

初始工作区在外部创建前保存固定意图。local 路径使用固定 Git worktree 和原生 Codex session；Orca 路径由 Orca 创建 worktree，核实唯一原终端并完成冻结 agent 启动命令；业务回合继续向同一 terminal 投递普通输入。重启核对原身份，不能按同名标题或路径推断替代会话。工作开始后，中心不会因反馈、dirty 文件、项目检查结果或 PR 出现而 reset、清理或改写 worker 的提交。

每次反馈有固定 Command ID 和 Operation。答案先在 SQLite 保存。local 运行中问题用仍有效的 app-server 工具请求 ID 回复，结束后反馈在原 thread 调用 `turn/start`；Orca 当前不提供原生提问/活动反馈路由，结束后反馈沿原 terminal 投递新业务输入，核验原请求的接受与开始事实。服务重开先核验旧执行和回执，旧的未送达输入不自动投递；用户须在统一 Decision 入口重新确认。已终止的 local worker 经核实后可用新受管进程和新 Attempt 恢复原 thread；原 thread 无法核实时需人工处理，不在原 Execution 内新建会话。平台接受回执不等于 agent 已处理。local 通知写入每轮 `appserver.jsonl`，完整正文由原生会话历史补读；Orca 保存必要状态/缺口及受理的完整 final，不复制完整终端过程。回执不明时不更换 ID 重发。详见[决策与反馈](decisions.md)、[当前 Orca 边界](../architecture/orca-native-worker.md)和[停服恢复 Spec](../spec/worker-service-recovery.spec.md)。

人工模式允许不同 Task 分别领取；自动模式仍一次占用一个活动执行。暂停派发只限制新领取，不中断当前 worker、待决策反馈或 PR 核验。

## PR 与交付

worker 或用户提供的 URL 只是线索。中心经配置的 provider 查询真实 PR，并核对 URL、目标仓库、base、head 仓库和当前 Execution 的工作分支。查询失败保留线索并重试；关闭未合并返回待决策，仍可继续原会话。后续向同一 PR 推送不要求重跑中心 verify/review。

`CompleteNativeTask` 必须核实 provider 的 merged 状态、合并时间与有效 merge commit，然后在一笔 SQLite 事务中重核 Task 版本、选定 WorkflowRun/Execution、PR 归属及整任务发布绑定。事务唯一写入 TaskDelivery、关闭 Task、完成 WorkflowRun 并释放 DAG 后继。重复轮询和迟到事件重用同一交付，不回退终态。错误 PR、普通 closed、worker 自述完成或用户文字不能替代真实合并。

`task discard` 保留原有未合并核验与外部清理事实；状态不明时不静默关闭可能仍在运行的资源。已丢弃任务的 PR 若后来真实合并，只记录交付冲突，不生成正常 TaskDelivery。

## 控制入口

`steerline task list/show/graph/export` 只读中心权威；validate/create/discard/add-predecessors 通过固定版本与请求 ID 操作。控制台或 CLI 的 `answer_decision` 共享 CommandService 提交路径。正式服务入口只运行 `worker-flow-v2`；旧五步的 retry、mark-stopped、task reopen 和项目步骤验证器没有当前 CLI 入口。旧状态保留供人工核实，当前入口不读取或迁移旧业务数据。


## 当前 Orca 环境与恢复事实

Orca 方案冻结 environment 路由、repository selector、CLI 和原生启动命令；认证仍属于目标 runtime 的保存连接，不进入业务封套。worktree/终端准备与业务输入分别保存 Operation 和回执。准备不成为任务成果，每个原终端同时只允许一个 SteerLine 业务输入在途；原输入未知时只对账，不再发下一输入。

每轮保存原请求受理/开始事实、发送前有效基线及原主代理身份。订阅提示与独立补查询提供只读事实；已观察的额外输入边界、身份或正文冲突持久保留。worker 重启恢复原基线/请求/查询时间，离线完成可结算一次，后续输入仍受恢复确认门槛约束。事件在 actor 提交事实后确认，adapter 不能写中心状态。

每次有界查询受 CommandTimeout 约束，它不是模型整轮执行期限。未知 native outcome、缺少完整回复和终端写入者缺证分别记录。等待 PR 合并的终端诊断不回退主阶段，已提交交付不被迟到观察重开。当前 codec 直接拒绝旧记录，不迁移；实际验证与限制见[补查询验收](../acceptance/orca-completion-query.md)。旧 supervised 环境准备/nonce/Dispatch/relay 的[历史记录](../acceptance/archive/completed/worker-readiness.md)不定义当前单终端路径。

## 本地 Windows sandbox 配置冻结

冻结的 FixedExecutionProfile 保存实施 key/version、有界配置封套以及原发布查询/Git/认证引用。Windows local Codex 使用固定 workspace-write、unelevated、on-request、auto_review；用户输入不包含 sandbox 或审批覆盖。Orca 的模型/权限由执行主机原生设置管理，不套用 local 参数。local PolicyResources 保存精确 workspace/必要 Git 根及 adapter 生成参数；原生 thread、endpoint、process generation 和 Orca runtime/terminal/incarnation/pane/provider 位于各自 binding envelope。恢复读取原冻结 Profile 与绑定，不用当前全局配置改写已开始的 Execution。格式版本见[当前格式来源](#当前格式来源)；格式变化直接拒绝旧数据，无迁移或回填。

Decision 的实现问题路由也复用同一 64 KiB 封套，以 answer purpose 固定原 WorkflowRun、Execution、Attempt、generation、turn 和 question；回答路由不能冒充配置或资源封套。

Task 丢弃只保存 stop_pending 原执行停止待办。中心没有 PR 关闭、工作区/分支删除或资源清理进度/结算；停止回执沿固定身份原子更新 Task/Execution/Attempt，未知结果保留原请求。PR/工作区历史定位仍由原 WorkflowRun/Execution 保存。图分页协议为 native-task-graph/v2、cursor v2，仅按项目读取，不支持状态/邻域过滤和专用文件导出。

候选只读快照、历史 WorkflowSummary 和 worker_state 变化字段使用 `workflow_stage/workflow_status`，由领域记录 `StepID/Status` 明确映射；不保留 `finish_task_status` 派生枚举。恢复检查同样直接携带流程阶段/状态。内部 stage_inputs/stage_results 的 step_id 及任务/执行/决策对象内 status 保持。见[状态命名 Spec](../spec/state-naming.spec.md)。
