# runtime v5 / control protocol v1 / result v1 数据契约

编码为 UTF-8，时间为 RFC3339 UTC。完整例子：[runtime](../examples/runtime.json)、[result](../examples/result.json)。结果的可执行 JSON Schema 与程序一起嵌入：[result.schema.json](../internal/steerline/result.schema.json)。

## 顶层

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| schema_version | integer | 当前仅接受 5；v1–v4 及未知版本明确拒绝，不迁移 |
| updated_at | timestamp | 最近一次成功发布快照的时间，含心跳写入 |
| heartbeat_at | timestamp | 最近成功发布的活跃时间；读者用于判断过期 |
| service.instance_id | string | 每次启动新的随机实例 ID，不等于 claimant |
| service.pid | integer | 服务进程 PID |
| service.status | string | running / stopping / stopped / error |
| service.mode | string | scheduler（正常轮询派发）/ retry-one（仅重试指定已有运行）/ maintenance（人工恢复命令） |
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

控制 revision 按 service.instance_id 隔离，每个新实例从 1 开始；不占执行槽的其他 PR 完成不会使派发版本失效。run.control_revision 跨实例保存，并覆盖工作目录、base、进程身份等恢复证据的变化。自动派发、显式启动、旧 retry 和决策续跑共用执行守卫；决策续跑可以使用为待决策预留的槽，但不能绕过 unknown、控制 checkpoint 或 blocker。

## 每项运行

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| task_id / title | string | Beads 任务身份和领取时标题 |
| run_id / claimant | string | 本次随机运行 ID / 固定领取者 |
| attempt_id | string | 此运行当前日志代次；首次执行等于 run_id，后续 retry 使用新 ID |
| control_revision | integer | 该运行最近一次操作相关状态变更的版本 |
| host | string | `local` 或 `orca`；缺失时拒绝继续服务 |
| completion | object/null | 派发时固定的 `{command:{executable,args},timeout}`；null 使用原生 close。重启按该记录继续，配置变更不能跳过旧运行的结项尾部 |
| beads_context | object/null | 派发时的 beads_dir、project_id、database、backend；回调前再次核对。旧记录可能为 null，不包含认证信息 |
| beads_status | string | 最近核实的 Beads 状态，不以本地预测替代 closed。这是过渡期的外部任务态镜像，见 [状态模型](state-model.md) |
| execution_status | string | pending / running / succeeded / failed / unknown |
| finish_task_status | string | pending / waiting_decision / waiting_pr_merge / completed / blocked |
| decision | object/null | 仅 outcome=needs_decision 时存在：{id, request, status(pending/resuming/applied), option_id, note, answered_by, answered_at, applied_at, request_path, answer_path}。见 [decisions](decisions.md) |
| codex_session_id | string/null | 从 thread.started.thread_id 得到，不从日志文字猜测 |
| process | object/null | 宿主无关身份：kind（os/orca）、PID/Created/Executable（os）或 Terminal/RuntimeID/Incarnation（orca）；kind 为空视为 os |
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

PR 4A 使用 runtime v5 当前格式。新执行身份、能力检查和外部动作回执保存在 `execution/v1/`：binding 路径为 `bindings/<run_id>/<attempt_id>.json`，Operation 路径为 `operations/<run_id>/<attempt_id>/<operation_id>.json`，能力报告在 `capabilities/`，内容寻址的原生证据副本在 `evidence/`。binding 不可变；Operation ID 绑定固定负载摘要，同 ID 异负载拒绝。持锁 Service 写执行记录；停服后的 `certify-backend` 维护入口取得同一仓库/run_dir 锁后才能导入能力报告。runtime 保留业务权威与执行投影；未知或矛盾状态保留槽位，不自动重发副作用。

准备 worktree 时 execution_status=pending；创建或接入检查失败为 failed/blocked；遗留 pending/running 为 unknown/blocked。重启只核对目录、共享 Git 元数据、分支和起点祖先关系，不重建、不重跑。实际工作目录等字段只在创建及技能/Beads 核验全部通过后写入；创建后检查失败时，目录可能已存在，应查看 workspace_intent 和 error。

缺少当前执行绑定、调度器准备记录或身份不完整的运行不能通过 accept-result 补造工作区事实，需保留原状态并人工核对；程序不生成虚构的准备证据。

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

终态回执暂时无法发布时，服务保留 completed checkpoint 并重试发布，继续监督已有 worker、响应停服；后续变更、决策续跑和 PR 结项先等待该 checkpoint 结算。runtime 自身持久化失败仍按既有规则停止服务。恢复时，缺失 applying 回执不能否认已保存的 run/领取证据；这种情况保留 indeterminate 与执行阻塞。完整历史请求只索引一次，既有终态回执不会因归档请求后来损坏而被重写。

## 候选任务快照

持锁服务在每个 task tick 发布 `tasks.json`，版本为 1，包含服务 instance、repository identity、查询时间、最近成功时间、错误、阻塞原因及候选列表。候选来自 Beads ready，排除不支持类型、aggregate 和已有本地 run；候选只用于展示，不作为授权或 claim 保证。provider 查询失败保留旧列表并记录错误，定向启动总会重新读取并核验 provider。CLI `steerline tasks` 只读取当前实例快照。

## runtime 版本拒绝

读取 v1–v4 时明确返回不支持，不改写文件。当前格式缺少 attempt_id、host 或固定执行身份时拒绝副作用，不从配置或日志路径补造。停服并核实执行身份后才能更换状态目录；不能用旧快照覆盖已经产生外部动作的当前状态。

## 执行结果

八个基础键都必须出现，不允许额外键：task_id、run_id、outcome、pr_url、worktree_path、base_commit、summary、error；此外 `decision` 是可选键（仅 outcome=needs_decision 时使用）。

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

运行记录不自动删除；日志保留策略见 README。外部读取器只能读 runtime，不写回；不能把这个文件当作队列或任务数据库。整个快照原子替换，不支持增量读取半个文件。

Windows 打开文件时建议允许 FILE_SHARE_DELETE；无法控制的读取库应重试短暂共享冲突。过期 heartbeat 意味着服务/磁盘可能异常，须显示状态过期，不能把最后一次 running 当成当前事实。重大版本改变通过 schema_version 区分。
