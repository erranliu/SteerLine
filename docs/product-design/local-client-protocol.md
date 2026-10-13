# 本机客户端协议 v1

本页描述 #45 基线与[任务 #91](../spec/tui-http-client.spec.md)当前源码。TUI 已统一消费 HTTP；实现/验收/合入事实归专属验收与 execution-map。

纯契约在 `internal/product/httpcontract`，客户端在 `httpclient`，受限文件能力在 `httpfs`；`httpapi` 提供服务端 handler，TUI 由 `consoleapp` 组装。服务继续拥有 Task、WorkflowRun、Execution、Attempt、Decision、Command、Operation 和唯一交付状态；HTTP 只映射现有 `product/api` DTO 与命令入口。

## 发现、连接和身份

服务先取得现有仓库/运行目录锁并完成恢复，再绑定 `127.0.0.1` 的随机 TCP 端口并开始提供 HTTP。服务不随 UI 窗口退出。实例发现文件位于 `<run_dir>/local-api/discovery.json`，凭证文件位于同目录的 `credentials/` 子目录；文件内容不进入 URL 或日志。Windows 目录和凭证 DACL 仅授予当前 Windows SID，客户端连接前重新核验 DACL。每个服务实例创建新凭证；发现文件包含 `protocol`, `project_id`, `repository_identity`, `instance_id`, `process`, `address`, `credential_path`, `ready` 与诊断，不包含凭证值。端口冲突也会写受限 `ready=false` 发现记录，保留原生绑定错误供客户端显示。

`project_id` 是仓库身份 UTF-8 字节 SHA-256 的小写十六进制路由键；完整仓库身份只通过认证的 identity DTO 返回。客户端不得通过 PATH、超时或旧发现文件启动另一服务。`ready=false` 返回原实例的启动诊断；此时只开放只读查询/回执及针对该实例的停服命令。

每个请求必须携带 `Authorization: Bearer <credential>`，使用精确发现的 `Host`，来源为 IPv4 loopback，且不包含浏览器 `Origin`。服务不启用 CORS。服务身份以 `/v1/identity` 响应中的协议、项目、实例、进程、能力和实时就绪状态为准。`ready=false` 时只提供诊断及只读查询，并允许停掉发现的原实例；业务变更被拒绝并以原 ID 保留 rejection。进程/实例不一致时客户端刷新并停止提交旧目标。

### Windows 连接样例

Windows 客户端从现有项目配置取得 `run_dir`，读取发现文件和凭证。以下 PowerShell 样例保持凭证在当前进程内存中；不要将它打印到日志、URL 或进程参数：

```powershell
$discoveryPath = Join-Path $runDir 'local-api\discovery.json'
$discovery = Get-Content -Raw $discoveryPath | ConvertFrom-Json
$token = [IO.File]::ReadAllText($discovery.credential_path)
$handler = [Net.Http.HttpClientHandler]::new()
$handler.UseProxy = $false
$client = [Net.Http.HttpClient]::new($handler)
$request = [Net.Http.HttpRequestMessage]::new([Net.Http.HttpMethod]::Get, "http://$($discovery.address)/v1/identity")
$request.Headers.Authorization = [Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token)
$request.Headers.Host = $discovery.address
$response = $client.Send($request)
$identity = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult() | ConvertFrom-Json
if ($identity.protocol -ne 'steerline-client/v1' -or $identity.project_id -ne $discovery.project_id -or
    $identity.instance_id -ne $discovery.instance_id -or -not $identity.ready) {
    throw "The discovered service is not the ready instance: $($identity.diagnostic)"
}
```

Go consumers can use `app.OpenHTTPClient(ctx, configPath)`; this only connects to the discovered instance and never starts one.

## 查询路由

所有项目查询以 `/v1/projects/{project_id}` 为根。任务/图的版本和游标沿用现有 DTO；cursor 绑定原快照/筛选条件，失效时客户端刷新整页，不拼接不同 revision。活动查询必须绑定原 WorkflowRun、Execution、Attempt。会话 final 查询返回正式正文引用；body 按原 revision、SHA-256、UTF-8 offset 有界读取。客户端用 `presentation` 查询现有 `ProjectPresentation`，展示 `DefaultProfileID`、profile 摘要及 `UnavailableReason`；命令只发送用户选择的 profile ID，服务仍通过原 `StartTaskPayload` 取得并核验配置 digest。

| 方法与路径 | DTO / 用途 |
| --- | --- |
| `GET /v1/identity` | `Identity`：协议、项目、实例、进程、能力、`ready`、诊断与认证后的两个原锁路径 `lock_paths` |
| `GET /v1/projects/{project_id}/snapshot?task_id=` | `api.ProjectSnapshot`：连接、服务运行、决策/历史和选中任务 |
| `GET /v1/projects/{project_id}/tasks?cursor=&limit=` | `api.TaskPage`：有界任务列表 |
| `GET /v1/projects/{project_id}/tasks/{task_id}` | `api.TaskPage`：任务详情、直接依赖、交付和阻断 |
| `GET /v1/projects/{project_id}/graph?cursor=&limit=` | `api.GraphPage`：有界基本 DAG |
| `GET /v1/projects/{project_id}/candidates` | `api.TaskCandidates`：当前可领取任务候选 |
| `GET /v1/projects/{project_id}/presentation` | `ProjectPresentation`：现有安全执行方案摘要、DefaultProfileID、配置 digest 和不可用原因，供客户端选择 start_task/start_candidate 的 profile ID |
| `GET /v1/projects/{project_id}/runs/{run_id}` | 精确 run 的 `api.CandidateSummary`，含候选、Decision、PR 与交付状态 |
| `GET /v1/projects/{project_id}/runs/{run_id}/activity?execution_id=&attempt_id=&cursor=` | `ActivityPage`，只读规范数据库活动，不回退私有日志 |
| `GET /v1/projects/{project_id}/runs/{run_id}/activity?execution_id=&attempt_id=&recent=true` | 同一原 Execution/Attempt 的有界近期活动投影；TUI 刷新使用，不能混用流式 cursor |
| `GET /v1/projects/{project_id}/runs/{run_id}/conversation?execution_id=&attempt_id=&session_id=&source=&turn_id=&finals_only=&cursor=&limit=` | 有界会话事件页和原正文引用 |
| `GET /v1/projects/{project_id}/runs/{run_id}/final` | `FinalReply`，返回 final 事件及不超过 4 MiB 的完整正文；更大的正文使用 body 路由分段读取 |
| `GET /v1/projects/{project_id}/runs/{run_id}/body?ref=<hex-json>&offset=&limit=` | `api.ConversationBodyPage`，原消息 revision/SHA 校验，单页不超过 256 KiB |
| `GET /v1/projects/{project_id}/runs/{run_id}/recovery?execution_id=&attempt_id=` | `api.RecoveryInspection`，只读原恢复核验，必须绑定当前 Execution/Attempt，不授予继续资格 |
| `GET /v1/projects/{project_id}/runs/{run_id}/decision-eligibility?execution_id=&attempt_id=` | 只读原决策资格核验，同样要求精确绑定 |
| `GET /v1/projects/{project_id}/commands/{request_id}` | 查询原 command/control 回执；没有权威记录才返回 `command_not_found` |

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
| `abandon_candidate` | run ID / candidate revision + control revision | `{"task_id":"...","expected_task_version":3,"reason":"..."}` | 原 `abandon_candidate` control |
| `discard_task` | task ID / task version | `{"reason":"..."}` | 原 `TaskCommands.Discard` 与持久 discard command/receipt |
| `set_dispatch_mode` | `dispatch` / control revision | `{"mode":"manual"}` 或 `{"mode":"auto"}` | 原控制命令与 authority CAS |
| `pause_dispatch`, `resume_dispatch` | `dispatch` / control revision | `{}` | 原控制命令与 authority CAS |
| `open_terminal`, `recheck_environment` | run ID / blocker version + control revision | `{"target":{"run_id":"...","execution_id":"...","attempt_id":"...","turn_id":"...","blocker_id":"...","version":2}}` | 原宿主 environment control |
| `open_native_session` | run ID / expected version 0 | `api.NativeSessionTarget`：run/Execution/Attempt/session/generation | 原冻结路径核验后打开原生查看器；持久标记 unknown 后只调用一次，无新业务轮次 |
| `stop_service` | 当前 `instance_id` / expected version 0 | `{}` | 原服务停服 control request；退出前在在途 HTTP 请求中等待原 applied 回执，不另启动服务 |

`create_task`, `add_predecessors`, planning、人工验收和自定义前置操作目前返回 `unsupported`。服务把有效 request ID 的 unsupported 拒绝保存在原 envelope 记录中；之后 GET 仍返回 `unsupported` rejection，不会伪装成 `command_not_found`。

命令外层 `status` 为 `submitted`, `accepted`, `applied`, `rejected`, `outcome_unknown` 之一。服务端原 control/command receipt 原样附带，内部 `applying` 表示 `accepted`，不表示 agent 已处理。`indeterminate` 或 `pending_verification` 映射为 `outcome_unknown`，客户端只查询原 request ID，不生成新 ID。

### 回执与故障恢复

`GET /commands/{request_id}` 对 HTTP control envelope 先读同项目原 control request/receipt（例如 dispatch control 同时会产生 SQLite `set_dispatch_state` 命令记录），再查项目 SQLite command authority；answer/discard 先查 SQLite，再查 control inbox。数据库缺失、权限、格式、身份或读取错误均返回诊断错误，不映射成不存在。HTTP 已登记但尚未派发的 envelope，只有底层两种权威都明确无记录且没有持久拒绝回执时才返回：

```json
{"code":"command_not_found","reason":"the project database and control request authority contain no record for this request_id","request_id":"0123456789abcdef0123456789abcdef"}
```

发送前客户端保存 `CommandEnvelope`。重连先 GET 原 ID：有回执则按原 target/revision 继续；`outcome_unknown` 则只对账，不重复调用 worker/backend；权威 `command_not_found` 才能按原完整 envelope、原 ID、原 payload 和原版本补交。已登记的 stale/permission/unsupported rejection 也有持久回执，重连不会把它误当成未发送。服务的 `local-api/commands/{request_id}.json` 是跨 HTTP 动作的持久唯一仲裁，底层 control inbox 与 SQLite command/discard ID 再各自做并发/事务唯一约束。连接失败、SQL 读取失败、旧实例和权限不足都保持各自错误码。

TUI 将完整原信封保存在 `<home>/ui/http-requests/<project_id>/<request_id>.json`，POST 前原子发布；该客户端记录不成为业务权威。对原生查看器，HTTP 持久结果只用于防止重复打开；未知结果不重新调用打开器。

停服以原 HTTP applied 回执、认证后的原 OS 进程 generation 退出和两个已有锁释放共同确认。客户端不读取宿主回执文件；缺证据时显示结果未核实。TUI 只结束客户端；断线保留本窗口观察与草稿，禁止变更，重新连接不会生成替代请求。

本协议不提供 SSE、离线查询进程、完整原始过程阅读器、规划 handler 或通用 artifact DTO。
