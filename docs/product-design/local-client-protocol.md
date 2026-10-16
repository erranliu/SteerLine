# 本机客户端协议

应用 v2 是当前产品的本机入口，Web 为唯一交互前端。`httpcontract` 的项目 DTO/信封、`httpapi` 的查询/命令处理与 `product/api` 用例保持原事务和资格规则；共享后端在进程内调用项目端口，不启动项目 HTTP listener 或通过回环 HTTP 转发。下表保留项目端口的逻辑路由；外部路径统一在 `/v2/application/projects/{project_id}` 下。

## 发现、连接和身份

`steerline serve` 发布 `<home>/application-api/discovery.json` 与私有 Bearer，协议为 `steerline-application/v2`。凭证、目录和记录仅当前用户可读，严格核对 home、实例、精确 OS PID/创建时间/exe、loopback Host 和在线 identity，拒绝 Origin、重定向与错实例。共享 PID 不代替各项目 runtime instance；项目和宿主的实例分别换代。业务事实仍由原项目 Task/WorkflowRun/Execution/Attempt/Decision/Command/Operation 和唯一交付拥有。

## 查询路由

所有项目查询以 `/v2/application/projects/{project_id}` 为根。任务/图的版本和游标沿用现有 DTO；cursor 绑定原快照/筛选条件，失效时客户端刷新整页，不拼接不同 revision。活动查询必须绑定原 WorkflowRun、Execution、Attempt。会话 final 查询返回正式正文引用；body 按原 revision、SHA-256、UTF-8 offset 有界读取。客户端用 `presentation` 查询现有 `ProjectPresentation`，展示 `DefaultProfileID`、profile 摘要及 `UnavailableReason`；命令只发送用户选择的 profile ID，服务仍通过原 `StartTaskPayload` 取得并核验配置 digest。

| 方法与路径 | DTO / 用途 |
| --- | --- |
| `GET /v2/application/projects/{project_id}/identity` | `Identity`：协议、项目、实例、进程、能力、`ready`、诊断与认证后的两个原锁路径 `lock_paths` |
| `GET /v2/application/projects/{project_id}/snapshot?task_id=` | `api.ProjectSnapshot`：连接、服务运行、决策/历史和选中任务 |
| `GET /v2/application/projects/{project_id}/tasks?cursor=&limit=` | `api.TaskPage`：有界任务列表 |
| `GET /v2/application/projects/{project_id}/tasks/{task_id}` | `api.TaskPage`：任务详情、直接依赖、交付和阻断 |
| `GET /v2/application/projects/{project_id}/graph?cursor=&limit=` | `api.GraphPage`：有界基本 DAG |
| `GET /v2/application/projects/{project_id}/candidates` | `api.TaskCandidates`：当前可领取任务候选 |
| `GET /v2/application/projects/{project_id}/presentation` | `ProjectPresentation`：现有安全执行方案摘要、DefaultProfileID、配置 digest 和不可用原因，供客户端选择 start_task/start_candidate 的 profile ID |
| `GET /v2/application/projects/{project_id}/runs/{run_id}` | 精确 run 的 `api.CandidateSummary`，含候选、Decision、PR 与交付状态 |
| `GET /v2/application/projects/{project_id}/runs/{run_id}/activity?execution_id=&attempt_id=&cursor=` | `ActivityPage`，只读规范数据库活动，不回退私有日志 |
| `GET /v2/application/projects/{project_id}/runs/{run_id}/activity?execution_id=&attempt_id=&recent=true` | 同一原 Execution/Attempt 的有界近期活动投影；近期查询使用，不能混用流式 cursor |
| `GET /v2/application/projects/{project_id}/runs/{run_id}/conversation?execution_id=&attempt_id=&session_id=&source=&turn_id=&finals_only=&cursor=&limit=` | 有界会话事件页和原正文引用 |
| `GET /v2/application/projects/{project_id}/runs/{run_id}/final` | `FinalReply`，返回 final 事件及不超过 4 MiB 的完整正文；更大的正文使用 body 路由分段读取 |
| `GET /v2/application/projects/{project_id}/runs/{run_id}/body?ref=<hex-json>&offset=&limit=` | `api.ConversationBodyPage`，原消息 revision/SHA 校验，单页不超过 256 KiB |
| `GET /v2/application/projects/{project_id}/runs/{run_id}/recovery?execution_id=&attempt_id=` | `api.RecoveryInspection`，只读原恢复核验，必须绑定当前 Execution/Attempt，不授予继续资格 |
| `GET /v2/application/projects/{project_id}/runs/{run_id}/decision-eligibility?execution_id=&attempt_id=` | 只读原决策资格核验，同样要求精确绑定 |
| `GET /v2/application/projects/{project_id}/commands/{request_id}` | 查询原 command/control 回执；没有权威记录才返回 `command_not_found` |

`TaskPage`, `GraphPage`, conversation event/body 和活动 cursor 保留各自实际身份、版本、采样/事件时间。PR、交付和历史 Decision 从现有查询投影返回，不使用 UI 缓存补写。conversation 的空 attempt 选择原 run/Execution 的完整历史；指定历史 attempt 时核对正式会话投影归属。body 保留原 Attempt/revision/SHA，不改向当前 Attempt。

## 命令映射

所有 POST 使用唯一的 32 位小写十六进制 `request_id`、当前 `project_id`/`instance_id`、精确 `target_id`、显示时的 `expected_version`，并按动作携带控制或图 revision。请求信封在发送前由调用方持久保存。服务将完整原信封先原子登记，再写入现有宿主 immutable control inbox 或调用 SQLite CommandService；同 ID 不同信封返回 `request_id_conflict`。服务端从当前 OS 用户取得 `requested_by`，忽略客户端不能证明的身份。控制请求绑定原实例/显示 revision，由运行服务再次核对实例、control/graph/task/decision 版本和领域资格；过期目标保留原 request 并拒绝，不转向当前最新对象。

POST JSON 正文上限为 1,048,576 bytes（1 MiB），超出返回 HTTP 413 / `request_body_too_large`。登记前还核对完整原信封、实际规范化映射意图、digest、创建时间及 JSON framing/newline 的聚合编码大小：不含结果摘要的记录上限 1,064,960 bytes，为有界结果预留 16 KiB；超出返回 HTTP 413 / `request_record_too_large`，该新 ID 不登记，不提交 control/SQLite 或触发业务副作用。各动作的既有字段/领域限制仍适用，正文在 1 MiB 内不代表映射后一定可准入。持久文件写入与正式读取共用 1,081,344-byte（1 MiB + 32 KiB）硬上限。

仲裁文件只保存一份完整原信封和完整原映射意图；结果摘要只保存 status、code、diagnostic 和采样时间，不重复保存 `original` 或 typed authority DTO。GET/同 ID 重交从原信封恢复身份与 `original`，并从正式 authority 查询完整 typed receipt；后续回执大小不扩张仲裁文件。authority 明确丢失时保留原信封并返回 `outcome_unknown`，不重放；原 rejection 保持 `rejected`。只有持久摘要的 code/diagnostic 限为各 1,024 UTF-8 bytes，超长文本保留前缀及 ` [truncated]`；正常 authority 查询输出仍保留完整回执。当前开发格式不迁移旧记录，不双读或回退。

| `action` | `target_id` / 版本 | `payload` | 现有入口 |
| --- | --- | --- | --- |
| `start_task` | task ID / task version + `expected_control_revision` | `{"execution_profile_id":"..."}` | `StartTaskPayload` → 原 `start_task` control；服务再次核对 task version、资格、实例与 revision |
| `start_candidate` | task ID / task version + graph/control revision | `{"execution_profile_id":"..."}` | 原 `start_candidate` control 与原 candidate command |
| `answer_decision` | decision ID / decision version；payload 绑定 run revision/attempt/recovery | `{"run_id":"...","decision_id":"...","option_id":"...","note":"...","answers":{},"expected_run_revision":7,"source_attempt_id":"...","expected_recovery_version":2}` | 原 `DecisionPort.Submit` 与 SQLite `CommandService` |
| `discard_task` | task ID / task version | `{"reason":"..."}` | 原 `TaskCommands.Discard` 与持久 discard command/receipt |
| `set_dispatch_mode` | `dispatch` / control revision | `{"mode":"manual"}` 或 `{"mode":"auto"}` | 原控制命令与 authority CAS |
| `pause_dispatch`, `resume_dispatch` | `dispatch` / control revision | `{}` | 原控制命令与 authority CAS |
| `open_terminal`, `recheck_environment` | run ID / blocker version + control revision | `{"target":{"run_id":"...","execution_id":"...","attempt_id":"...","turn_id":"...","blocker_id":"...","version":2}}` | 原宿主 environment control |
| `open_native_session` | run ID / expected version 0 | `api.NativeSessionTarget`：run/Execution/Attempt/session/generation | 原冻结路径核验后打开原生查看器；持久标记 unknown 后只调用一次，无新业务轮次 |

`create_task`, `add_predecessors`, planning、人工验收和自定义前置操作目前返回 `unsupported`。服务把有效 request ID 的 unsupported 拒绝保存在原 envelope 记录中；之后 GET 仍返回 `unsupported` rejection，不会伪装成 `command_not_found`。

命令外层 `status` 为 `submitted`, `accepted`, `applied`, `rejected`, `outcome_unknown` 之一。服务端原 control/command receipt 原样附带，内部 `applying` 表示 `accepted`，不表示 agent 已处理。`indeterminate` 或 `pending_verification` 映射为 `outcome_unknown`，客户端只查询原 request ID，不生成新 ID。

### 回执与故障恢复

`GET /commands/{request_id}` 对 HTTP control envelope 先读同项目原 control request/receipt（例如 dispatch control 同时会产生 SQLite `set_dispatch_state` 命令记录），再查项目 SQLite command authority；answer/discard 先查 SQLite，再查 control inbox。数据库缺失、权限、格式、身份或读取错误均返回诊断错误，不映射成不存在。HTTP 已登记但尚未派发的 envelope，只有底层两种权威都明确无记录且没有持久拒绝回执时才返回：

```json
{"code":"command_not_found","reason":"the project database and control request authority contain no record for this request_id","request_id":"0123456789abcdef0123456789abcdef"}
```

发送前客户端保存 `CommandEnvelope`。重连先 GET 原 ID：有回执则按原 target/revision 继续；`outcome_unknown` 则只对账，不重复调用 worker/backend；权威 `command_not_found` 才能按原完整 envelope、原 ID、原 payload 和原版本补交。已登记的 stale/permission/unsupported rejection 也有持久回执，重连不会把它误当成未发送。服务的 `application-commands/{request_id}.json` 是跨 HTTP 动作的持久唯一仲裁，底层 control inbox 与 SQLite command/discard ID 再各自做并发/事务唯一约束。连接失败、SQL 读取失败、旧实例和权限不足都保持各自错误码。

Web 将完整原信封保存在现有 IndexedDB；服务端的原请求记录仍是跨动作仲裁。原生查看器的未知结果不得重放。

公开 stop_service action/capability 已退役，不映射为全局停机；旧原请求结果仍可只读查询。关闭浏览器不停止业务。

本协议不提供 SSE、离线查询进程、完整原始过程阅读器、规划 handler 或通用 artifact DTO。

## 应用入口与浏览器转接

`GET /v2/application/identity` 无项目也可用；`GET /v2/application/projects` 返回冻结配置快照与 starting/ready/failed/stopping/stopped 投影。目录查询不建库；项目装载先通过只读身份 census/冲突核验并取得仓库及运行目录锁，随后初始化原项目。配置变更在下次宿主启动生效。

各项目 identity、查询及 commands 经目录直接取得进程内端口；保留原 project/runtime instance、原 request ID、版本与完整 JSON 数字。缺失、冲突或未 ready 返回明确不可用；只有原权威缺失才返回 command_not_found，不用 POST 重放查证。

CLI 通过项目选择访问原任务用例：POST `/task-batch-validation`、`/task-batches`、`/tasks/{task}/predecessors`；原结果 GET `/task-batches/{request_id}` 和 `/tasks/{task}/predecessors/requests/{request_id}`。这些窄路由只接受原生认证，不进入 Web AllowedRead 或浏览器写入白名单。任务事务由原项目 actor 串行办理，CLI 不开第二写者。

`POST /v2/application/shutdown` 先关闭全部项目新输入资格再确认原宿主 identity，按项目聚合正常关闭与资源结算。私有 `shutdown.json` 绑定原宿主，stop 同时核验此结果、精确进程退出与租约；缺失、损坏、旧实例或任何未核实项均失败。本地关闭未知保留库/锁；worker 未知但本地已关时可重开原库取证，禁止替代 worker 或新业务输入。

Web 只提供受限 `/api/v2/application/projects…` 同源转接；不代理管理/CLI 专有路由。浏览器会话绑定 home/Web 实例，原生 `/native/identity`、`/native/backend`、`/native/shutdown` 使用独立 launcher Bearer，无 Origin，浏览器 cookie 不能停服。Web 可独立重启，重新 open 授权后只读重连；原信封仍按原实例/请求查询。见[Web 指南](../../web/README.md)、[统一启动 Spec](../spec/unified-startup.spec.md)。


## 独立环境检查

沿同一项目根提供 `GET /environment-checks/options`（预填/机制能力）、`GET /environment-checks`（本仓历史）与 `GET /environment-checks/{key}`（原请求公开快照/离线缓存）。这些查询不启动服务、Agent 或清理，不要求创建 Task；原命令查询只 inspect 该请求，详情在无未知操作时按可信 check_id 查询。start/stop 复用当前项目持有的原 Runner 实例准入，检查入口不重启 daemon。报告保留 Runner 的源时间、revision、结论和资源状态，读取时间不能冒充新检查时间。

`POST /commands` 新动作 `start_environment_check`：target_id 为项目 project_id、expected_version=0，payload 为 `{profile_id,access_mode}` 或 `{execution,access_mode}`。access_mode 必须为 sandbox/full_access；后者由用户明确选择。`stop_environment_check`：target_id 为原 start 的 32 位 key、expected_version 为原生 report.revision，payload `{}`。二者不使用图/控制 revision；不会修改执行方案或业务权限。

原请求在同一仲裁 namespace 中登记，SQLite 另存绑定与公开快照；`environment_check` 为完整查询投影，仲裁文件仍仅存小型结果摘要。applied 表示命令已确认受理，执行/能力/资源状态见报告。环境检查原写结果未知即只读原 ID，即使 command_not_found/request_not_found 也不补交 POST；同 ID/信封的重复 POST 从原绑定取回结果，不重派，包含 transport 仲裁文件丢失而领域绑定仍在的情形；即使本地绑定提交失败也不以新 ID 补发。尚无可信 check_id/revision 时不能 stop；原 stop 未核实或同 revision 不能再次派发收尾。细节与 Runner 发布前置见 [Spec](../spec/environment-check.spec.md)。
