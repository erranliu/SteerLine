# 本机客户端协议

本页维护当前应用 v2 的连接、查询、命令和恢复约束；业务资格分别见[数据模型](data-model.md)、[反馈与恢复](decisions.md)、[项目维护](project-maintenance.md)和[Runner 接入](../integration/steerrunner.md)。Web 是当前交互前端；应用在进程内调用各项目端口，不启动项目 HTTP listener。

## 发现、连接和身份

原生客户端读取当前用户私有的 `<home>/application-api/discovery.json` 和 Bearer，核对 `steerline-application/v2`、home、实例、OS PID/创建时间/exe 与在线 identity。只接受精确 loopback Host，拒绝 Origin 和重定向；凭证不进入浏览器。宿主与项目实例分别换代，共享 PID 不代替项目身份。

项目路由根为 `/v2/application/projects/{project_id}`。应用目录从启动时冻结的配置装载；列表查询不建库，项目须经过身份冲突核验和持锁装载。未 ready、项目冲突或身份不符返回明确诊断，不能当作记录不存在。

## 查询路由

下表路径均相对项目根；仅 GET。完整白名单由[共享路由定义](../../internal/product/httpcontract/routes.go)维护，字段由[公共契约](../../internal/product/httpcontract/contracts.go)及原产品 DTO 定义。

| 路径 | 用途 |
| --- | --- |
| `/identity` | 项目/实例、进程、ready、能力、诊断及认证后的原锁路径 |
| `/tasks`、`/tasks/{task_id}`、`/graph`、`/snapshot`、`/candidates` | 任务、DAG、项目快照与可领取任务 |
| `/presentation` | 安全的 profile 摘要、默认项、配置 digest 和不可用原因 |
| `/activities` | [异步活动中心](async-activities.md)的有界只读来源聚合与原请求关联，不调用 Git/Runner 或推进业务 |
| `/activities/{activity_id}` | 精确活动摘要；仍只读本地事实和有界消息预览 |
| `/activities/{activity_id}/interactions` | 显式读取已记录交互一页，默认 50、最多 200、1 MiB；游标绑定项目/实例/活动/原成员范围与读取版本，变化失效后显式刷新 |
| `/activities/{activity_id}/changes` | 原持久 revision 与读取索引版本组成的轻量标记，无交互或正文；不推进分页 |
| `/activities/{activity_id}/body` | 按 item、sha256、offset 显式读取该条原正文一段，最多 256 KiB；原正文上限 16 MiB，原归属/摘要/UTF-8 校验及应用级有界内存复用 |
| `/runs/{run_id}` | 精确候选、参与/提交状态、Runner 观察、PR 与交付 |
| `/runs/{run_id}/activity`、`/last-message`、`/usage` | 原执行的活动、最近消息和用量 |
| `/runs/{run_id}/conversation`、`/final`、`/body` | 公开会话事件、final 和原正文分段 |
| `/runs/{run_id}/recovery`、`/decision-eligibility` | 当前 Execution/Attempt 的只读资格核验，不授予继续权限 |
| `/runs/{run_id}/tests`、`/documents` | 候选固定测试安排/进度与文档版本、差异 |
| `/tests/catalogue`、`/tests/records`、`/tests/records/{id}`、`/tests/generations/{id}` | 清单、固定测试记录和原生成详情 |
| `/maintenance`、`/maintenance/readiness`、`/maintenance/log`、`/maintenance/commits/{sha}`、`/maintenance/tasks/{task_id}` | 项目同步、统一就绪资格、提交及材料详情 |
| `/environment`、`/environment/options`、`/environment/materials`、`/environment/{id}` | 环境选项、材料、活动摘要和原活动详情 |
| `/roadmaps`、`/roadmaps/{id}`、`/roadmaps/sources/{id}` | [Roadmap 读取与转正](roadmap-promotion.md)的专用投影 |
| `/commands/{request_id}` | 原请求回执；仅权威明确无记录才返回 `command_not_found` |

分页保留原身份、revision、筛选条件和 cursor，失效后刷新原查询，不拼接不同快照。activity 的近期查询 `recent=true` 不混用流式 cursor。conversation 空 attempt 读取原 Execution 的完整历史，指定历史 attempt 则核对正式投影归属；recovery/decision-eligibility 必须绑定当前 Execution/Attempt。

final 最多直接返回 4 MiB 正文；更大内容按原引用从 body 读取，单页最多 256 KiB，核对原 Attempt、revision、SHA-256 和 UTF-8 offset。来源规则见[公开会话结果](execution-observability.md#公开结果来源)，不回退原生日志。

候选的 participation、submission_status、stop_requested 与 Runner workflow_status 分别表达原事实；有公开 Projection 时保留 Runner Status，无投影时使用提交状态。winner/loser 只从同一快照 TaskDelivery 产生，历史缺交付上下文时不补造。人工退出参与保留 reason/by/ended_at，不等于 Runner 停止；业务与资源边界见[生命周期](worker-lifecycle.md)。

准备等待沿原候选 `runner_delivery.execution_wait` 返回完整 `steerrunner-execution-wait/v1` 对象或 null，activity 的 `kind=execution_wait` 保留 `meta.execution_wait`。只展示，不新增计时命令；读取异常保留最后已知值并标待核实。字段及验证范围见[等待 Spec](../../SteerlineFlow/spec/runner-wait-retry.spec.md)。

## 命令映射

`POST /commands` 使用 32 位小写十六进制 request_id、当前 project_id/instance_id、精确 target_id、expected_version，以及动作要求的 control/graph revision。客户端发送前持久保存完整原信封；服务登记原信封与首次规范化的固定意图，再交给原控制入口/项目事务。同 ID 不同信封返回 `request_id_conflict`，过期请求拒绝且保留原回执，不改投最新对象；requested_by 由服务取得，不信任客户端自报身份。

| action | 目标、版本与 payload |
| --- | --- |
| `start_task` | task ID / task version，control revision；`{execution_profile_id}` |
| `start_candidate` | task ID / task version，graph/control revision；`{execution_profile_id}` |
| `stop_candidate`、`resolve_pr_conflict` | candidate ID / candidate version，control revision、graph revision 必须为 0；`{task_id}`。前者退出参与，后者沿原冲突处理入口 |
| `answer_decision` | decision ID / version；`{run_id,decision_id,option_id,note,answers,expected_run_revision,source_attempt_id,expected_recovery_version}`，核对当前问题及原执行身份 |
| `discard_task` | task ID / version；`{reason}`，原因不能为空 |
| `set_dispatch_mode` | dispatch / control revision；`{"mode":"manual"}` 或 `{"mode":"auto"}` |
| `pause_dispatch`、`resume_dispatch` | dispatch / control revision；`{}` |
| `open_terminal`、`recheck_environment` | run ID / blocker version，control revision；`{target:{run_id,execution_id,attempt_id,turn_id,blocker_id,version}}`，只作用于原 blocker |
| `open_native_session` | run ID / 0；原 `NativeSessionTarget` 的 run/Execution/Attempt/session/generation。只打开原查看器，无新业务轮次 |

以上启动动作仍受项目统一就绪和原任务/资源资格约束。通用 commands 不支持 create_task、add_predecessors、planning、人工验收和自定义前置；有效 ID 的拒绝持久保存为 unsupported，而非 not_found。任务创建另走下节原生窄接口。公开 stop_service 已退役，旧回执只读保留；关闭浏览器不停止业务。

POST 正文最大 1,048,576 bytes。登记前检查原信封、固定意图、digest、时间与 JSON framing 聚合编码：无结果摘要最多 1,064,960 bytes，为结果预留 16 KiB；持久读写硬上限 1,081,344 bytes。超限分别返回 HTTP 413 / request_body_too_large 或 request_record_too_large，新 ID 不登记、零业务副作用。持久摘要的 code/diagnostic 各限 1,024 UTF-8 bytes，超长保留前缀与 ` [truncated]`；正式 authority 回执保持完整。仲裁文件只存一份原信封、意图与紧凑结果，typed receipt 从原权威查询，不复制膨胀。

三条受理顺序由同一命令仲裁保护：普通命令先解析映射和大小校验、登记、重读，再核对能力/就绪/目标并派发固定意图；环境动作严格校验信封与端口后登记，再作领域核验；原生查看器核实原会话后先持久化 outcome_unknown，再仅调用一次。未知查看器不得重开。当前实现与验证见[读取/命令边界验收](../../SteerlineFlow/testing/acceptance/product-read-command-web-refactor.md)。

### 回执与故障恢复

公开 status 为 submitted、accepted、applied、rejected、outcome_unknown；内部 applying 映射 accepted，indeterminate/pending_verification 映射 outcome_unknown。applied 说明相应命令生效，不证明模型完成、PR 合并或资源停止。

原请求仲裁同时核对 journal、control inbox 和 SQLite 权威；HTTP control 优先查原 control，answer/discard 优先查 SQLite。权限、数据库、格式、身份和读取错误保留诊断，不映射成不存在。原权威曾受理而后缺失时保持 unknown；原 rejection 保持 rejected。

- 重连先 GET 原 ID，核对项目、实例、目标、版本及完整原信封；未知只读对账，不换 ID。
- 仅从未收到任何回执、明确允许补交的动作，且同实例两层业务权威均明确无记录时，才能按原完整信封、ID、payload 和版本补交。候选停止、环境与项目维护动作只对账，不自动补交。
- 一旦已有 submitted/accepted/outcome_unknown 或 `request_receipt_persist_failed`，后来 not_found 也不能自动 POST。
- 回执保存失败时，Web 保留原信封与诊断，立即在 5 秒预算内 GET 核实；确认 applied/rejected 后据实显示。超时、暂缺、读取或身份问题保留“提交结果待核实”，继续原号查询。
- 浏览器 journal 保存在 IndexedDB；它不替代服务端权威，也不允许跨实例补交旧请求。

## 应用入口与浏览器转接

`GET /v2/application/identity` 无项目可用；`GET /v2/application/projects` 返回冻结项目目录与装载状态。原生应用管理还包括 usage-collection 和 shutdown，均不因存在原生入口而自动开放给浏览器。

原生任务接口为 POST `/task-batch-validation`、`/task-batches`、`/tasks/{task}/predecessors`，原结果 GET `/task-batches/{request_id}`、`/tasks/{task}/predecessors/requests/{request_id}`；沿项目 actor 和原任务事务办理，不开第二写者，也不进入 Web 通用白名单。

原生 shutdown 先关闭全部项目新输入，再按原宿主身份聚合关闭结果。私有 shutdown.json、精确进程退出与租约共同核验；缺失/损坏/旧实例不算正常停机。恢复与部署归[安装部署](../integration/install-deploy.md#同构建更新与重启)。

Web 只作受限同源 `/api/v2/application/...` 转接；原生 `/native/identity`、`/native/backend`、`/native/shutdown` 使用独立 launcher Bearer，无 Origin。浏览器 cookie 不授予停服或 CLI 管理权限。独立 Web 重启后重新 open 授权并只读重连；认证和配置专用接口见[Web 指南](../../web/README.md)。统一宿主的契约与证据见[统一启动 Spec](../../SteerlineFlow/spec/unified-startup.spec.md)。

## 项目环境适配

查询 environment/options 返回环境键、方案、当前引用 revision、就绪原因和历史入口；materials 返回所选环境材料与固定输入诊断；environment 可按 environment_key 筛选最近 20 条，environment/{id} 返回原活动详情。GET 不执行脚本或启动 Agent；长回复节选明确标注，全文保留在原 Run。

| action | target_id / expected_version | payload |
| --- | --- | --- |
| `environment_start` | project_id / 1 | `{environment_key,profile_id,round_timeout_seconds,expected_current_revision}` |
| `environment_invalidate` | project_id / 1 | `{environment_key,expected_current_revision,reason}` |
| `environment_stop` | 原活动 ID / revision | `{}` |

动作不使用图/控制 revision。开始/失效的 expected_current_revision 必填，无当前引用时为 0；它不代替活动 revision。当前仅单次普通 Run，round_timeout_seconds 为正整数秒，无自动多轮重试；environment_generate 已删除。

环境结果以原 Run 绑定、停止结算、封存的 result-file、脚本报告及 verified 成功 final 共同核验；进度/回复或命令 applied 不单独授予 ready。最终结果 closed/0 保留采集失败，closed/1 才有文件，封存后不重采。失效只阻断新候选，未知按原号核实，不因 not_found 补交。当前单次执行和准入以[服务实现](../../internal/environmentadaptation/service.go)及[模型](../../internal/environmentadaptation/model/model.go)为准；[适配 Spec](../../SteerlineFlow/spec/project-environment-adaptation.spec.md)记录原交付范围。

检测详情和历史的 `blocked` 是原 Run 阻断事实的只读投影，`timed_out` 表示整次检测已超时；两者不改写 `state`、`settled` 或停止资格。未就绪时，顶层诊断优先保留原活动原因，缺少说明时读取当前轮及原 Run 诊断，首次提交错误继续保留。未结算阻断在环境页显示“检测阻断”，活动中心列入进行中及需要关注；指定检测 ID 的详情沿原号轮询，不自动停止或重试。

## 项目维护动作与能力边界

维护动作沿原项目 actor、命令 journal 和原请求回执，不使用图/控制 revision。

| action | target_id / expected_version | payload |
| --- | --- | --- |
| maintenance_check、maintenance_update、maintenance_generate_all | project_id / 1 | `{}` |
| maintenance_profile | project_id / 1 | `{profile}` |
| maintenance_retry | task_id / 1 | `{task_id}` |
| maintenance_stop_generation | generation_id / 1（当前客户端） | `{generation_id}`；request_id 使用外层原号 |

生成登记不等于材料已就绪；完整流程归[项目维护](project-maintenance.md)，动作定义归[领域路由](../../internal/app/maintenance_routes.go)。Roadmap 动作见[专用协议](roadmap-promotion.md)，不另维护一份转正字段表。

本协议没有 SSE、离线查询进程、完整原始过程阅读器或通用 artifact DTO。旧 Runner Check 路由/动作及投影已删除，不提供兼容；项目脚本通过普通 Run 执行，Runner 不承担检测业务。退役范围见[Spec](../../SteerlineFlow/spec/legacy-environment-check-retirement.spec.md)与[验收](../../SteerlineFlow/testing/acceptance/legacy-environment-check-retirement.md)。精简前版本及补充证据从[历史来源](../../SteerlineFlow/archive/design/runner-reliability-history.md#第五批整理来源)追溯。
