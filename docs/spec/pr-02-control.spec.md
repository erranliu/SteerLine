# PR 2 Spec：服务控制协议与定向执行

状态：实现完成，待 PR 审核。依赖：PR 1（已合并）。代码包含本 Spec 的 C1–C7；自动恢复动作仍按范围限制留待后续 PR。对应 [Plan](pr-02-control.plan.md)。

## 目标

把服务从“启动后持续自动派发”扩展为可接收明确控制指令的单写者。交付可由 CLI 验证的定向任务启动、派发模式、暂停/恢复、停止请求以及持久回执，为控制台提供统一接口。

## C1：模式与执行槽

| 模式/状态 | 自动领取 | 显式 start_task | 当前 worker | 已提交决策的续跑 | PR 核验/结项 |
| --- | --- | --- | --- | --- | --- |
| manual，未暂停 | 否 | 允许，须核验 | 继续 | 允许，须核验 | 继续 |
| auto，未暂停 | 允许 | 拒绝，提示先切手动 | 继续 | 允许，须核验 | 继续 |
| 任一模式，已暂停派发 | 否 | 拒绝 | 继续 | 允许，须核验 | 继续 |
| stopping/stopped | 否 | 拒绝 | 进入既有 shutdown / 已停止 | 否 | 停止轮询 |

- 新增 `run -dispatch manual|auto`，默认 auto，兼容现有脚本。每次启动采用本次参数，不隐式恢复上次 auto。
- manual 下启动与 task tick 仍做认领对账、刷新候选；只有收到 start_task 才走新任务领取。
- 暂停只阻止新执行的派发（包括 PR 5 的 retry），不挂起进程。模式变更不会自动解除暂停。
- 槽占用统一计算：准备中、worker 存在、待决策保留、未解决 unknown、未确定的执行类控制指令。任何入口必须使用相同守卫。
- 指令由服务主状态机串行执行；UI、CLI、心跳 goroutine 都不能直接修改 `Service.State`。

## C2：定向启动

`start_task(task_id)` 的成功含义为：该任务已领取、工作目录完成核验、指定 worker 已启动且运行身份持久化。它不代表任务交付完成。

1. 验证目标实例、控制版本、manual/未暂停和执行槽。
2. 重新读取 Beads ready 与目标任务详情；验证目标仍在 ready 集合、类型可执行、未分配、非聚合任务、没有已有本地 run。
3. 固定这次指令的 run_id，复用原有 fetch、领取前持久化 intent、原子 claim、worktree 准备与启动路径。
4. 若目标不再可用，返回明确原因，绝不改选 ready 列表里的其他任务。
5. 领取冲突、准备失败及领取结果未知，分别沿用既有记录/保留现场策略；回执关联准确的 run_id。失败后不能自动以新 ID 重发。

自动模式通过同一个“执行所选候选”的函数工作，只有选择候选的方式不同。claim 仍是最终并发仲裁，列表上的 ready 不是领取保证。

## C3：本机文件控制协议 v1

目录均在该配置的 run_dir 内：

```text
control/requests/<request_id>/request.json  客户端发布，一经发布不可改
control/receipts/<request_id>.json          持锁服务写入，可推进状态
tasks.json                                持锁服务发布的候选快照
```

请求发布必须具备原子可见性和“同 ID 不可覆盖”语义。可用按 ID 的发布锁加原子重命名；`AtomicWrite` 的可替换写入本身不能提供这条保证。只有完整 request.json 才进入消费范围，临时文件/空目录不算请求。

请求字段：

| 字段 | 规则 |
| --- | --- |
| protocol_version | 固定 1；不支持的版本拒绝 |
| request_id | 32 位小写 hex，路径与内容必须相同 |
| repository_identity | 已解析的 Git common-dir，必须匹配当前服务 |
| instance_id | 精确目标实例；所有在线控制指令必填 |
| action | 本 PR：start_task / set_dispatch_mode / pause_dispatch / resume_dispatch / stop_service |
| expected_control_revision | 除 stop_service 外必填，校验针对控制状态；后续 run 操作用 expected_run_revision |
| run_id / decision_id / expected_run_revision | run 操作预留的协议字段，本 PR 的新任务动作不用 |
| payload | 动作专用严格结构，例如 task_id 或 mode；拒绝未知参数 |
| requested_by | 操作者；默认沿用现有环境中的回答者名称规则 |
| created_at / expires_at | UTC 时间；客户端默认有效期 2 分钟，服务受理前检查；受理后不因到期中断正在执行的动作 |

路径由服务根据 request_id 推导，不能使用请求中的任意输出路径；结果接回的内容载荷在 PR 5 定义。请求上限 3 MiB，兼容最大 2 MiB 的结果文件及外层封装。此通道面向已有本机文件访问权限的操作者，不新增远程权限模型。

回执包含 version、request_id、请求内容摘要、instance_id、action、status、code、message、操作者、各阶段时间、目标 run/attempt/decision 以及相关状态版本。UI 的 submitted 状态由已发布请求推导，不等于服务回执。

| 回执 status | 精确定义 |
| --- | --- |
| applying | 服务已校验并持久化控制操作 checkpoint，正在执行 |
| applied | 该动作定义的完成条件已持久化；任务后续执行结果另外查看 run |
| rejected | 前提条件不满足，无该动作的业务副作用 |
| failed | 动作失败，已知副作用已记录；可能已领取或留下 worktree |
| indeterminate | 崩溃/通信结果等导致无法确认副作用，要求核对与人工恢复 |

错误 code 至少覆盖 stale_instance、stale_revision、expired、slot_busy、dispatch_paused、wrong_mode、task_not_ready、already_managed、claim_conflict、host_unsupported、invalid_request、persistence_failed。message 供人阅读，客户端逻辑依据 code。

同 ID、同内容的再次提交只查询原回执；同 ID 不同内容报冲突。服务多个扫描周期、客户端超时和 UI 重启都不能导致同一指令再次启动 worker。不同 ID 针对同一任务也受既有 run 与槽守卫约束。指令是即时动作请求，忙时拒绝，不保留为待执行任务队列。

## C4：持久化与崩溃边界

- 服务在产生业务副作用前，把 `control.operation` 写入 runtime：request_id、action、phase=applying、预分配目标身份、动作参数摘要。
- 同时只允许一个会改变业务状态的 operation。主循环收到的新请求按明确顺序校验；UI 不依赖跨客户端请求到达顺序。
- 完成时把业务结果和 operation 的 phase=completed、终态回执内容一起原子保存到 runtime，再发布外部回执，最后清理 checkpoint。发布终态回执之前不接下一个变更动作。
- 如果终态回执写入失败，保留 completed checkpoint，停止新的业务变更并报告持久化异常；仍可处理停服信号。不能绕过它继续派发。
- 重启后 completed checkpoint 可重建回执。applying checkpoint 在无法从既有持久证据证明结果时标为 indeterminate，按既有恢复规则保留 unknown/blocked；不重放 claim、worktree create、agent start 或 terminal send。
- applying 的 start_task 即使尚无完整 run，也必须保留预分配 run_id 和控制阻塞原因，禁止再开始任务，直至核验副作用并解除。未知的执行副作用写入 `control.blockers` 后才能清理 operation checkpoint；阻塞不妨碍查询、停服和专门的恢复动作。PR 5 提供对应的人工核验入口。
- 前实例未受理的请求在新实例标为 stale_instance；到期请求为 expired。重启不能把它们变成新授权。
- 只读日志不是恢复证据；`activity.jsonl` 丢失不能改变指令执行次数。
- 本期保留请求/回执历史，不加入自动清理；扫描器在内存索引已完成请求，不能每个 tick 重新解析全部历史。

## C5：runtime v4 与版本守卫

沿用现有执行/交付状态轴，新增以下字段，PR 3–5 共用此结构：

| 字段 | 用途 |
| --- | --- |
| service.process_identity | 服务本身的 OS 进程身份，含创建时间和可执行文件，供 PR 3 探测 |
| service.startup_id | 本次启动握手 ID，可空（手动前台 run） |
| service.config_digest | 启动时实际读取的配置文件原始字节 SHA-256；不包含环境变量中的凭据值 |
| service.ready | 已完成初始化、持锁且可处理指令；stopping/stopped 时 false |
| service.control_protocol | 支持的控制协议版本 |
| service.capabilities | 已实现的动作名称集合；新增动作不等于新增 runtime 字段 |
| service.heartbeat_interval_ms | 实际心跳间隔，供客户端计算新鲜度 |
| control.dispatch_mode / dispatch_paused | manual/auto 与暂停开关 |
| control.revision | 当前实例的控制版本，从 1 开始 |
| control.operation | 上述单条正在执行/待发回执的 checkpoint，空闲为 null |
| control.blockers | 未确定执行副作用的持久阻塞列表，记录 request_id、action、task_id、run_id、attempt_id、decision_id（后两项可空）和 reason；由显式恢复动作解除 |
| runs[].control_revision | run 的操作相关版本，跨实例保存 |
| runs[].attempt_id | 首次执行取 run_id；retry 取新 ID，关联日志代次 |

checkpoint 的可选目标字段在本版一次定义：run_id、attempt_id、decision_id、action、request_id、phase、request_digest、result。result 承载已知终态回执数据，不复制日志。

控制版本只随模式/暂停、执行槽与执行阻塞条件变化推进；心跳、日志和无关 PR 查询时间变化不推进。run 版本只随执行/交付转换、attempt、决策、PR 绑定或恢复事实变化推进；重复轮询无变化不推进。同一事务涉及多项变更时只推进一次。

旧快照由持锁服务升级：缺失 revision 从 1 开始；attempt 从已记录的 attempts 路径取 ID，否则取 run_id。恢复转换仍需推进版本。只读客户端不得迁移文件。

## C6：候选任务与能力

- 每个 task tick，无论 manual/auto/paused，刷新 `tasks.json` v1，包含 instance_id、repository_identity、queried_at、last_success_at、error 和候选数组。
- 列表来自 Beads ready；过滤类型、聚合任务和已有本地 run，提供 task_id、title、type 及展示所需摘要。保持任务提供方适配边界，UI 不直接调用 Beads。
- provider 查询失败保留旧列表并标记错误；启动动作仍需重新读取且失败时拒绝，不能使用缓存领取。
- 空闲但被 waiting_decision、unknown 或 operation 阻塞时给出可读原因；不能只返回笼统的 slot busy。
- cheap eligibility 可从当前快照计算；调用实际动作时由服务重新执行完整校验。UI 按钮可用性不是授权证据。

## C7：CLI 与停止

- 提供 `steerline control -action <action>` 及动作参数；客户端读取实例/版本后构造不可变请求，返回 request_id，可等待回执或再次按 ID 查询。UI 将复用同一个 Go 客户端，不通过 shell 拼参数。
- `stop_service` 只要求目标实例匹配，避免版本变化阻止停服。保留 `steerline stop` / stop.json 兼容路径。
- stop 在 fetch、worktree 准备或 completion 命令执行时仍可触发 context 取消；辅助 watcher 只触发取消，runtime 和回执仍由持锁状态机/退出收尾写入。
- stop 可以中断当前 applying 动作，不占用第二个并行 operation。收尾先把被中断动作记为有证据的 failed 或 indeterminate 并保存必要 blocker，再推进 stop 的回执；不能用 stop 的 checkpoint 覆盖尚未记录的执行副作用。
- 已进入 stopping 后拒绝其他新变更动作。stop applied 在最终 stopped 状态持久化之后发布；若崩溃或持久化失败，客户端显示未确认并通过实例探测核验，不能猜测成功。

## 验收标准

| ID | 场景与结果 |
| --- | --- |
| C-A1 | manual 启动并经过多个 task tick，候选刷新但 claim/start 次数为零 |
| C-A2 | ready 有 A/B，显式启动 B；只领取 B；B 失效时不会运行 A |
| C-A3 | 同指令重复提交、扫描重入和两个不同指令抢槽，至多启动一个 worker |
| C-A4 | 心跳/日志更新不使操作版本失效；真实状态变化使旧命令被拒绝 |
| C-A5 | pause/resume、模式切换遵守 C1；worker、决策等待和 PR 轮询不被暂停开关误停 |
| C-A6 | 在 checkpoint、claim、worktree、worker、终态落盘与回执之间注入崩溃，无自动重复执行；未知结果可见且阻止新派发 |
| C-A7 | 旧实例、到期、同 ID 改内容及错误仓库请求被拒绝，无任务副作用 |
| C-A8 | stop 能取消长准备操作，按原 shutdown_grace 收尾，回执不提前称 stopped |
| C-A9 | v1–v3 读入后在持锁保存时升级 v4，关键运行/PR/claim 证据保留 |
| C-A10 | CLI 默认 run 仍自动调度，既有生命周期、恢复、PR 结项测试通过 |

## 范围限制

本 PR 不启动后台进程、不实现新 UI。answer/retry/mark-stopped/accept-result 的在线动作在 PR 5 增加；服务 capability 在本 PR 只宣告已实现的动作。
