# runtime v5 / control protocol v1 / result v1 数据契约

编码为 UTF-8，时间为 RFC3339 UTC。完整例子：[runtime](../examples/runtime.json)、[result](../examples/result.json)。结果的可执行 JSON Schema 与程序一起嵌入：[result.schema.json](../internal/steerline/result.schema.json)。

## Native task query `native-tasks/v1`

配置显式设为 `"task_authority":"native"` 后，`steerline task list/show/graph` 读取中心 SQLite 中的 native Task、固定契约、依赖、WorkflowRun 和已记录交付；`task validate/create/discard` 通过中心命令服务处理固定批次、原子入图和显式丢弃。所有入口接受 `-config`，不调用 `bd show`。新项目首次使用时初始化 native 项目权威，初始 `dispatch_mode=manual`、`dispatch_paused=true`。本阶段没有生产 worker/技能接入，原生领取服务只预留 workflow/执行身份并将 `start_task_applied=false`；真实工作区启动仍等待 [Beads 退出 Spec](spec/beads-exit.spec.md) 的生产接入验收。

只读 native 查询配置示例：[native-tasks.config.json](../examples/native-tasks.config.json)。省略 `run_dir`，让它按 Git common-dir 派生为本仓库唯一状态目录。

native 状态目录必须使用仓库唯一派生的 `STEERLINE_HOME/state/<repoKey>`，不得为同一仓库配置另一条 `run_dir`。目录首次初始化前必须为空，仓库不得含 Beads 状态。SQLite 会把 Git common-dir、目标远端 URL、remote 名称和 branch 固定到项目权威；状态、仓库或配置不匹配时以 `authority_mismatch` / `unsupported_schema` 拒绝，不创建第二份数据库。

查询响应头含 `protocol`、`project_id`、`authority_version`、`graph_revision`、`database_id`、`database_revision`、`sampled_at` 和 `capabilities`。任务详情含原数字 `task_id`、固定契约与验收引用、状态/claim/priority/version、前置/后继、`dependency_ready`、未满足前置状态、`startup_blockers`、丢弃审计/清理待办、相关运行和交付证据。依赖和启动阻塞在同一 SQLite 读取快照内计算；`discarded_predecessor` 与 `inconsistent_delivery` 保持阻塞。读取前会核对交付行与合并证据、离页前置的 payload 与状态列；损坏记录不成为 ready 或领取许可。暂停、占槽和本阶段尚无生产执行接入分别报告为 `dispatch_paused`、`execution_slot_busy` 和 `native_execution_unavailable`，不混入依赖状态。列表默认 100、最多 500 条，数据库内按数字 ID 排序分页；`next_cursor` 固定数据库身份、项目、authority/graph/database 修订和 offset。续页发现数据库被替换、修订变化或版本不可读时返回 `cursor_invalidated`。依赖就绪、依赖阻塞和启动阻塞能力均为 `provided`。已有项目查询不竞争仓库写锁，仍在每个读取快照中核对配置绑定。

`task-batch/v1` 是创建命令 payload，未知字段、缺 task_id 的任务、非法类型/优先级、无效固定契约及重复/悬空/跨项目/成环依赖均拒绝。`task validate -file <json>` 只读验证并返回 authority/graph/database 采样版本、节点错误和环路径；`task create -file <json> -request-id <id> -expected-authority-version <n>` 将批次、Task、契约、依赖、事件、graph_revision 和成功回执在同一事务提交。任务 ID 沿用输入整数；同 request_id 同内容返回原回执，不同内容返回 `idempotency_conflict`。创建不启动任务，回执说明 auto 且未暂停时后续正常轮询可以选择新任务。

`task discard -task-id <id> -request-id <id> -expected-version <n> -reason <text>` 先持久化绑定任务/运行/PR 的意图，再在数据库事务外核验精确 PR。合并状态不明或发布结果身份不完整时返回 `pending_verification` 并保留 Operation；真实未合并后才 CAS 为 discarded。已合并任务拒绝丢弃。依赖边、claim、运行和 task_id 保留；被丢弃的前置不会放行后继。活动执行、开放 PR 或工作区未清理时，详情保留 `cleanup_pending` 并显示人工待办。已丢弃任务的精确 PR 后来确认合并时，只追加幂等的 `task.delivery.conflict` 事件，不生成 TaskDelivery。

`task graph` 返回带相同数据库及 graph revision 的节点、边、交付来源、阻塞原因和一致性游标。关系读取只投影中心 SQLite 状态，不维护第二套图或复制依赖判定。

TaskContract 使用单个 Spec、单个配套 Plan、至少一个含验收章节的 Markdown 引用，以及只指向共享 constraint/protocol 文件的 `common_refs`。文件名不要求具有同一前缀，每个引用分别固定仓库相对路径、完整 commit 和该提交中普通文件 blob 字节的 SHA-256。验收引用不能再加入其他任务的 `.spec.md` / `.plan.md` 或固定文件的另一版本；配套关系和语义充分性由规格审阅负责。路径不能越界或是符号链接。规范摘要是 UTF-8、对象键排序、数组顺序不变、无多余空白的 JSON SHA-256；重复 JSON 键拒绝。契约内容从 Git commit blob 读取，工作区 CRLF 转换不改变核验结果。

数据库当前 schema 为 3，复用中心表与 Task 身份建立新库。已有 schema 1/2 或未知格式不自动迁移；版本号及表、索引、触发器定义都必须匹配当前格式。依赖表约束拒绝环和修改/删除已有边。TASK-DAG 负责原生入图、ready/claim、discard 和 delivery 事务；BEADS-EXIT 仍负责把这些事务接入生产原生 worker 与真实平台生命周期。

TASK-DAG 更新后的协议样例见 [native-tasks-v1.sample.json](validation/task-dag/native-tasks-v1.sample.json)；TASK-CORE 原始基线样例与验收记录保留在 [task-core validation](validation/task-core/README.md)。

## 顶层

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| schema_version | integer | 当前投影版本 5；只接受 v5。v1–v4 明确拒绝，不读取、不转换，也不删除原文件 |
| revision | integer | SQLite 业务数据库 revision；投影落后时由 QueryService 查询数据库 |
| tasks / workflow_runs / executions / attempts / decisions | arrays | 任务、流程、执行、尝试及决策的稳定 ID 关联读模型；旧 `runs` 保留为兼容展示摘要 |
| projection_error | string | 最近一次可见读投影导出错误；SQLite 已提交状态不会因投影写失败回滚 |
| updated_at | timestamp | 最近一次成功发布快照的时间，含心跳写入 |
| heartbeat_at | timestamp | 最近成功发布的活跃时间；读者用于判断过期 |
| service.instance_id | string | 每次启动新的随机实例 ID，不等于 claimant |
| service.pid | integer | 服务进程 PID |
| service.status | string | running / stopping / stopped / error |
| service.mode | string | scheduler（正常轮询派发）/ maintenance（生命周期初始化或人工恢复） |
| service.claimant | string | 稳定 Beads 领取者，用于恢复不完整记录 |
| service.error | string/null | 最近服务级错误，查询恢复后可能清除 |
| service.process_identity | object/null | 服务 OS 进程身份；含 PID、创建时间和可执行文件，供连接器核验实例 |
| service.startup_id | string/null | 后台启动握手 ID；前台运行为空 |
| service.config_digest | string | 实际加载的配置文件原始字节 SHA-256，不含环境变量凭据 |
| service.ready | boolean | 已完成初始化、持有仓库和运行目录锁并可受理控制指令；停服时为 false |
| service.control_protocol | integer | 当前支持的控制协议版本，本版为 1 |
| service.capabilities | string[] | 当前二进制已实现并可受理的动作，不代表后续阶段能力 |
| service.heartbeat_interval_ms | integer | 服务实际心跳间隔；观察端据此计算新鲜度 |
| repository.path | string | 解析后的仓库绝对路径 |
| repository.remote / branch | string | 明确的目标远端和主线 |
| repository.identity | string | 规范化 Git common-dir，用于跨 worktree 锁与恢复校验 |
| current_task | string/null | 当前准备或执行的候选任务；fetch 阶段可能尚未领取，没有则 null |
| slot_occupied | boolean | 主线获取/worktree 准备/worker 占用，或为待决策、unknown 旧执行、未结算控制操作及副作用 blocker 保留；保存时从这些事实重算 |
| runs | array | 本服务管理的全部运行概要，初始为空数组 |
| control | object | 派发模式、暂停、控制版本、单条 operation checkpoint 和未决副作用阻塞 |

`control.dispatch_mode` 为 `manual` 或 `auto`。每次 `run` 默认 auto，也可用 `-dispatch manual` 显式启动手动模式；启动参数生效，不隐式恢复上次模式。`dispatch_paused` 只阻止新派发，当前 worker、决策续跑和 PR 核验继续。`revision` 随模式/暂停、执行槽或执行阻塞变化；心跳、日志和重复 PR 轮询不推进。`control.operation` 是服务主状态机正在执行或等待发布终态回执的 checkpoint。`control.blockers` 保留不确定的 claim/worktree/worker 等执行副作用，阻止新任务派发，直到后续人工恢复动作解除。

控制 revision 按 service.instance_id 隔离，每个新实例从 1 开始；不占执行槽的其他 PR 完成不会使派发版本失效。run.control_revision 跨实例保存，并覆盖工作目录、base、进程身份等恢复证据的变化。自动派发、显式启动、在线 retry 与决策续跑共用执行守卫；决策续跑可以使用为待决策预留的槽，但不能绕过 unknown、控制 checkpoint 或 blocker。

## 每项运行

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| task_id / title | string | 外部来源任务身份和领取时标题；`domain_task_id` 是本地稳定 Task 身份，`identity_map_digest` 记录采用的身份映射 |
| run_id / claimant | string | 本次随机运行 ID / 固定领取者 |
| attempt_id | string | 此运行当前日志代次；首次执行等于 run_id，后续 retry 使用新 ID |
| control_revision | integer | 该运行最近一次操作相关状态变更的版本 |
| host | string | `local` 或 `orca`，与 backend/protocol 固定身份并存 |
| completion | object/null | 派发时固定的 `{command:{executable,args},timeout}`；null 使用原生 close。重启按该记录继续，配置变更不能跳过旧运行的结项尾部 |
| beads_context | object/null | 派发时的 beads_dir、project_id、database、backend；回调前再次核对。旧记录可能为 null，不包含认证信息 |
| beads_status | string | 最近核实的 Beads 状态，不以本地预测替代 closed。这是过渡期的外部任务态镜像，见 [状态模型](state-model.md) |
| execution_status | string | pending / running / succeeded / failed / unknown |
| finish_task_status | string | pending / waiting_decision / waiting_pr_merge / completed / blocked |
| decision | object/null | 当前问题和答案的展示摘要；权威版本、问题历史与答案保存在 SQLite `decisions`。状态为 pending、answer_saved、resuming、reply_accepted、applied、failed 或 superseded。见 [decisions](decisions.md) |
| codex_session_id | string/null | 从 thread.started.thread_id 得到，不从日志文字猜测 |
| process | object/null | 宿主无关身份：kind（os/orca）、PID/Created/Executable（os）或 Terminal/RuntimeID/Incarnation（orca）；缺少 kind 的执行绑定拒绝使用 |
| manual_stop | object/null | 人工停止确认：固定 attempt_id、confirmed_at、操作者与确认依据；仅结算该 attempt 的资源不确定性，不改写原 Operation 的未知结果。retry 创建新 attempt 时清除 |
| started_at / ended_at | timestamp/null | 服务记录的执行起止时间；崩溃导致终止时间未知时仍为 null |
| workspace_intent | object/null | 创建前持久化的 path、branch、base_commit；只表示计划创建，不证明目录已就绪。旧记录或领取孤儿可能为 null |
| worktree_path / worktree_branch / base_commit | string/null | 调度器创建并核验的隔离目录、实现分支及起点 Git hash；准备成功前均为 null，不由 Codex 结果覆盖 |
| pr.url | string/null | 当前运行的精确 PR 关联 |
| pr.state | string | open / merged / closed_unmerged / unknown |
| pr.merged_at / merge_commit | timestamp/string/null | 平台核实的合并证据 |
| pr.queried_at / query_error | timestamp/string/null | 最近查询时间和错误；失败时 state=unknown |
| last_event | string/null | 最近 JSON 事件的类型摘要，最多 512 字符，不存完整对话 |
| log_path / result_path | string/null | 每次运行独立的事件和结果文件；`activity.jsonl` 为 log_path 同目录的归一化派生物，不参与恢复；恢复孤儿记录时可能为 null |
| summary / error | string/null | 技能结果摘要（最多 4096 字符）/ 当前阻塞或失败原因 |

`process.created` 在 Windows 为进程创建 FILETIME 的十六进制标识，Linux 为 boot ID 和 /proc starttime。PID 可复用，读取器不能仅凭 PID 判定存活。遗留进程的核验结论写入 error；unknown 不代表服务已接管或恢复该会话。

execution_status=succeeded 仅表示进程正常退出，仍可能 finish_task_status=blocked（结果缺失/无效）、waiting_decision（等待人工决策）或 waiting_pr_merge。只有重新核实对应 PR 确实合入目标主线、配置的结项命令成功返回匹配回执（若有），且 Beads show 返回 closed，才写 completed。任务已 closed 但回调失败时仍 waiting_pr_merge，重启后重跑幂等回调。finish_task_status 不是“finish 技能函数是否返回”。

PR 4A 的执行后端复用 Spec 02 的 SQLite 权威存储：`execution_bindings` 保存不可变 ExecutionRef，`execution_operations` 保存后端动作的固定输入摘要和回执，与领域 `operations` 一起保存在 `steerline.db`。`execution/v1/` 只保留 `capabilities/` 能力报告及 `evidence/` 原生证据副本；不再读取文件式 binding/Operation。检测到旧文件式执行状态时拒绝启动并保留现场。持锁 Service 写执行记录，停服后的 `certify-backend` 取得同一仓库/run_dir 锁后维护能力报告。runtime v5 是数据库投影；未知或矛盾状态保留槽位，不自动重发副作用。

能力报告的 `platform_version` 固定平台 CLI 版本。Orca 使用本地 Codex CLI 覆盖启动时，还保存 `agent_version` 并纳入报告键和证据校验；原生 agent 模式的报告不能复用到该路径。

## 会话事件与正文读取

PR 4B 的 `ConversationEvent` 是 4C 控制台读取会话事实的公共 DTO。它绑定 project/workflow/execution/attempt、backend/protocol/target、source segment 与 session/turn/item/source event 身份；每种身份声明 native、derived 或 unknown。事件类型分别记录消息完成、轮次终态、宿主等待、来源缺口和结构化执行结果。`execution.result` 中的 summary 是执行摘要，正文引用不表示 assistant final。

事件、source cursor、去重身份、raw 记录和不可变正文 revision 使用 `steerline.db` 的 SQLite 事务提交。`ReadConversation` 的 Query cursor 绑定项目和完整过滤条件；`ReadMessageBody` 绑定 execution、attempt、item key 和指定 revision；`ReadRaw` 接受受管 raw 引用，不接受客户端路径。事件页最多 1000 条/1 MiB，正文页最多 256 KiB 且按 UTF-8 边界分页，raw 页最多 1 MiB。详细 JSON 和相位/结束语义见[Conversation events v1](conversation-events.md)。

数据库 schema 4 在当前 schema 3 的原生任务表与约束上增添会话表；升级在同一事务提交。更早的开发数据库版本仍被拒绝。

准备 worktree 时 execution_status=pending；创建或接入检查失败为 failed/blocked；遗留 pending/running 为 unknown/blocked。重启只核对目录、共享 Git 元数据、分支和起点祖先关系，不重建、不重跑。实际工作目录等字段只在创建及技能/Beads 核验全部通过后写入；创建后检查失败时，目录可能已存在，应查看 workspace_intent 和 error。

v5 中未带 workspace 准备证据的 Run 不能通过 accept-result 冒充已核验的运行；需要人工处理，并保留原有外部任务和执行证据。

PR 查询失败会清除 state 的可信结论并写 unknown，但保留过去读到的合并字段用于诊断；读者必须同时看 state、queried_at 和 query_error。程序不会仅凭缓存的 merged_at 关闭任务。

## 控制请求与回执

控制文件位于同一 `run_dir`：

```text
control/requests/<request_id>/request.json
control/receipts/<request_id>.json
tasks.json
```

客户端使用完整 32 位小写十六进制 request ID，通过同目录临时文件加原子 create-if-absent 发布 request.json。相同 ID、相同内容再次发布只查询原回执；同 ID 不同内容冲突，不覆盖原请求。空目录和临时文件不会被服务消费。服务只处理完整请求，并以经过校验的请求目录 ID 推导回执路径；请求正文中的错误 ID 不能指定另一个输出文件。单请求上限 3 MiB。

请求协议固定 `protocol_version=1`，包含 `repository_identity`、精确的 `instance_id`、`action`、`expected_control_revision`、严格动作 payload、`requested_by`、`created_at` 和 `expires_at`。客户端默认两分钟有效期；服务在受理前核验仓库、实例、版本和期限。`stop_service` 仍核验协议、实例、仓库、payload 与受理期限，但不依赖 ready 或易变化的控制 revision。停止请求可以取消初始化，受理后也不会因清理期间超过 expires_at 而丢弃。未知字段、缺失必需 payload 或不匹配的路径 ID 均拒绝；无参数动作允许省略 payload 或使用空对象。

本阶段支持的动作和 payload：

| action | payload | 条件与完成含义 |
| --- | --- | --- |
| `start_task` | `{"task_id":"..."}` | 仅 manual/未暂停/空槽；重新读取 ready 与详情后领取该任务，定向 worker 启动并身份落盘后为 applied |
| `set_dispatch_mode` | `{"mode":"manual"}` 或 `{"mode":"auto"}` | 切换之后的新任务选择方式；不会自动解除暂停 |
| `pause_dispatch` | 无 | 阻止自动领取和显式新启动，不暂停已有 worker、决策或 PR 查询 |
| `resume_dispatch` | 无 | 解除暂停并按当前 manual/auto 模式继续 |
| `stop_service` | 无 | 停止派发，等待 shutdown_grace 并收尾当前 worker；只有 stopped 状态保存后回执才为 applied |

回执状态：`applying` 表示服务已持久化 checkpoint 并开始动作；`applied` 表示动作完成条件已落盘；`rejected` 表示前提不符、没有任务副作用；`failed` 表示动作失败且已知副作用已记录；`indeterminate` 表示重启或通信中断后无法证明结果。常用 `code` 包括 `stale_instance`、`stale_revision`、`expired`、`slot_busy`、`dispatch_paused`、`wrong_mode`、`task_not_ready`、`already_managed`、`claim_conflict`、`host_unsupported`、`invalid_request`、`persistence_failed`。客户端按 code 分支，不解析 message。

runtime 中的 `control.operation` 先于副作用保存；完成时把业务结果和终态 receipt 一起保存为 completed checkpoint，再原子发布 receipt，最后清除 checkpoint。发布 applying receipt 失败时不派发。重启可重建 completed 回执；applying 操作不会重放 claim、worktree create、worker start 或 terminal send，不确定副作用进入 blockers。停服不能覆盖未结算操作的 evidence。控制历史不会自动清理，日志删除不影响指令次数。

### SQLite service command queue

人工回答与恢复动作都经当前 `CommandService` 追加到 `commands` 表；服务离线时客户端只读检查状态，不写请求文件或启动维护 Service。command payload 顶层固定包含 `service_instance_id`、`expected_revision`、`run_attempt_id`（resolve command 可省略）和动作专属 `body`。answer_decision 的 `expected_version` 绑定 DecisionRecord 版本；恢复动作的 expected revision 绑定 Run，resolve_command 绑定 control revision。新服务实例会拒绝旧实例尚未受理的 pending 命令。

| command.kind | body 主要字段 | accepted 的持久事实 |
| --- | --- | --- |
| `answer_decision` | decision_id、run_id、option_id、note、expected_run_revision、expected_decision_version、service_instance_id | SQLite 的 DecisionRecord 与 command 回执同事务记录固定答案；`accepted` 只表示答案已保存 |
| `retry_run` | 操作者确认与依据 | 同 task/run/worktree 的新 attempt worker 已启动且固定身份已保存 |
| `mark_stopped` | `confirmed_process_stopped`、`confirmation_basis` | 精确 unknown attempt 已标为 failed/blocked；Service 仍运行，旧 Operation 仍保留未知 |
| `accept_result` | 原结果字节 base64、SHA-256、停止确认依据 | SHA-256 匹配的原字节被保存到 `runs/<run_id>/recovery/<command_id>/result.json`，真实 PR 查询通过并进入 waiting_pr_merge |
| `resolve_command` | 指定 blocker 的人工核验依据 | 只解除该 blocker 并记录原 request_id / resolved_by；不会重放原指令 |

恢复命令先持久化为 `in_flight`。重启后绝不自动执行第二次；若结果未能通过保存的事实对账，则命令变为 `indeterminate` 并保留 control blocker，由后续 resolve_command 逐条处理。结果接回以 command payload 携带的冻结字节为准，不在服务执行时重读任意客户端路径。命令 `accepted` 不代表 PR 已合并或任务已完成。

上表 `start_task` 描述现有 Beads 控制生命周期。原生 `CommandService.ClaimNativeTask` 在中心事务中预留 Task claim、WorkflowRun、Execution、prepare Attempt、命令回执和 Operation；其响应为 `claim_reserved` 且 `start_task_applied=false`。只有生产原生接入完成工作区准备并持久化实际启动身份后，才能把控制命令标为 applied。原生 delivery 由 `CompleteNativeTask` 在事务外查询精确 PR，并在事务内重核 task version、所选运行、步骤门槛和整任务的 PR 发布绑定后写 TaskDelivery/closed；另一笔发布结果未知时即使选用的 PR 已合并也不关闭任务。

终态回执暂时无法发布时，服务保留 completed checkpoint 并重试发布，继续监督已有 worker、响应停服；后续变更、决策续跑和 PR 结项先等待该 checkpoint 结算。runtime 自身持久化失败仍按既有规则停止服务。恢复时，缺失 applying 回执不能否认已保存的 run/领取证据；这种情况保留 indeterminate 与执行阻塞。完整历史请求只索引一次，既有终态回执不会因归档请求后来损坏而被重写。

## 候选任务快照

持锁服务在每个 task tick 发布 `tasks.json`，版本为 1，包含服务 instance、repository identity、查询时间、最近成功时间、错误、阻塞原因及候选列表。候选来自 Beads ready，排除不支持类型、aggregate 和已有本地 run；候选只用于展示，不作为授权或 claim 保证。provider 查询失败保留旧列表并记录错误，定向启动总会重新读取并核验 provider。CLI `steerline tasks` 只读取当前实例快照。

## runtime 版本守卫

新服务只从 当前 SQLite 数据库恢复，并将 `runtime.json` v5 用作可重建观察投影。v1–v4 文件保持原样，`LoadRuntime` 和服务启动均返回 `unsupported runtime schema`；不会尝试从这些文件恢复、迁移或清理数据。v5 投影缺少 SQLite 数据库时也会拒绝恢复。已有规划数字 ID 的新任务通过 `run_dir/task-identity-map.json` 中经过项目、来源三元组和一对一校验的 `task-identity-map/v1` 映射取得稳定身份；不能按标题猜测。未映射的 Beads 任务使用不占数字规划号的 `legacy:<hash>` 身份。

## 执行结果

### minimal-v1 步骤结果

`project_steps.verify` 必须是带 `required_checks` 的确定性 command provider。命令从 stdin 读取版本化输入，通过 stdout 返回唯一 JSON 回执；检查失败可作为结构化检查结果返回，包装程序退出 0 表示回执完整，非零退出按步骤基础设施失败处理。步骤入口必须是固定 base 提交中的普通文件。内置 agent 入口只提供 implement/review 提示，publish_pr 须显式配置项目入口或命令。

WorkflowRun 保存步骤定义及 Codex executable/args、sandbox、model、reasoning_effort、profile。配置变更不改变已创建流程的执行参数。输入包含工作区证据、前置结果 ID 和正文，修复步骤能够读取实际失败检查或审查结果。

步骤回执包含 schema_version、project_identity、task_id、workflow_run_id、execution_id、attempt_id 与 result。中心逐项核验身份、输入 revision、真实 Git common-dir/分支/base、前置结果和步骤门槛。无效结果将尝试记为 failed；通过门槛后才提交 succeeded 与下一步意图。publish_pr 的前置验证/审查在启动发布器之前检查，PR head 在关闭外部任务之前核验。

minimal-v1 当前拒绝本地决策续跑：answer_decision 在接受前核验固定执行能力，不支持的 local continuation 会被拒绝并保留 pending 问题，不启动新步骤。若执行状态已有 `reply_accepted` 决策，结果仍须同时返回 `result.applied_decision_id`、`applied_decision_attempt_id`、`applied_decision_execution_id` 和 `applied_answer_sha256`，分别匹配原问题、decision attempt、Workflow Execution 和固定答案摘要；后端接受输入只算 reply_accepted，缺少证明时决策失败且 run 阻塞。单段 accept-result/retry/mark-stopped 入口不能用于 minimal-v1；不确定步骤必须按原尝试和 Operation 对账。

### 单次入口结果

八个基础键都必须出现，不允许额外键：task_id、run_id、outcome、pr_url、worktree_path、base_commit、summary、error；此外 `decision` 和 `decision_application` 是可选键。`decision` 仅用于 outcome=needs_decision；answer_decision 的后续结果只有携带严格匹配原 decision/attempt/execution/答案摘要的 `decision_application` 才能证明应用。

- task_id/run_id 必须与此次派发精确匹配。
- outcome 允许 pr_submitted、blocked、needs_decision。
- needs_decision 必须带 `decision` 对象：`question`、1–12 个 `options`（每个含 `id`、`label`，可选 `detail`），以及可选 `context`/`recommended`/`allow_free_text`。服务会置 `finish_task_status=waiting_decision` 并等待人工回答，见 [decisions](decisions.md)。
- pr_submitted 必须提供 PR URL，并原样回传调度器提供的绝对工作目录、40 或 64 位小写十六进制 base commit；不能用最后的实现 HEAD 替代 base。summary/error 可为 null。
- blocked 可将未知字段设为 null，应在 error 中解释原因。
- 最多读取 2 MiB，拒绝缺失字段、额外字段、多个 JSON 值、Markdown 包装或自然语言。

Codex 通过 `exec --json --output-schema <file> --output-last-message <per-run-result> ... -` 启动，提示词经 stdin 传入。事件 stdout 与结果文件独立；结果提交不等于任务完成。

进程工作目录为调度器创建的 worktree。接收结果或人工 accept-result 时，同时核验目录对应同一 Git common-dir、仍在分配的实现分支上，且 HEAD 保留 base_commit 作为祖先；核验失败即阻塞，不接受结果中的路径或提交改写事实。blocked 结果可用 null，但提供的目录/起点同样必须与调度记录一致。

PR 命令适配器的数据结构与 result 不同，见 README 的 PR 查询部分。命令查询是可信的本机适配边界，必须直接验证平台事实。

completion-request.json 与 completion-result.json 是每次运行目录下的最近一次结项请求和回执。服务通过 stdin 传请求，读取 stdout，非零退出时也保存返回的部分写入证据；这两份文件不会包含 PR 认证 token。详情见 [结项命令协议](completion-command.md)。completion 和 beads_context 是 v2 的可空扩展字段；不要用旧二进制读取新程序写入的快照。

显式 retry 不改变 task_id/run_id/worktree/base_commit。log_path/result_path 指向本次 attempts/<attempt_id> 目录；该目录的 previous-run.json 保留上一次运行概要，原日志和结果不移动、不覆盖。重试准备状态同样先落盘，意外中断后仍变成 unknown，不能自动恢复。service.mode 是 v2 的新增可选读取字段。

## 历史与读取规则

运行记录不自动删除。外部读取器不写回 runtime；查询与答案从当前数据库读取，runtime 导出损坏或失败可以重建。SQLite 同时限制每个 Task 的活动 WorkflowRun、每个 WorkflowRun 的活动 Execution，以及同一 Execution 的活动 StepAttempt。缺少这些当前约束的数据库明确拒绝，不自动升级。

Windows 打开文件时建议允许 FILE_SHARE_DELETE；无法控制的读取库应重试短暂共享冲突。过期 heartbeat 意味着服务/磁盘可能异常，须显示状态过期，不能把最后一次 running 当成当前事实。重大版本改变通过 schema_version 区分。
