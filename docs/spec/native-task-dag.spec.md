# Spec：原生任务、固定 DAG 与 Beads 退出

状态：待实施。

实施计划：[Plan](native-task-dag.plan.md)。共同约束：[执行架构](execution-architecture.spec.md)。基础：[中心执行模型](02-execution-model-and-adapter.md)、[双后端](pr-04a-execution-backends.spec.md)、[会话](pr-04b-conversation-events.spec.md)；展示接入：[控制台](pr-04-console.spec.md)。受监管远端仅在 [远程闭环](03-remote-execution-and-viewer.md) 的能力与验收范围内接入。

## N1：范围与交付顺序

SteerLine 自主管理任务身份、规格契约、依赖、领取和交付。最终必须在不安装 bd/bv、不设置 Beads 环境的项目中，从创建任务走到真实 PR 合并核验、任务关闭和后继就绪。

本 Spec 分四个交付阶段，名称是目标标识，不是 PR 编号：

| 阶段 | 范围 | 开工与集成前置 |
| --- | --- | --- |
| TASK-CORE | 原生任务身份、固定契约、项目权威、来源映射与查询 | 中心执行模型的 Task/WorkflowRun、SQLite、单写者和 Query/Command 已交付；沿用实际版本扩展 |
| TASK-DAG | 草稿校验、原子发布、固定硬前置、就绪/领取、交付释放和恢复 | TASK-CORE 验收通过并合入 |
| BEADS-EXIT | 原生配置与技能接入、worker/结项去耦、新项目闭环及旧项目迁移 | TASK-DAG 验收通过并合入；切换前结清该项目旧执行与回调 |
| DAG-VIEW | 图查询、TUI 关系详情、DOT/JSON 和可选 SVG 导出 | TASK-DAG 与控制台查询边界已交付；配套 Plan 默认在 BEADS-EXIT 后集成 |

原生任务不等待整个远程闭环完成。新建原生项目的生产执行在 BEADS-EXIT 验收前不启用；前两阶段使用隔离项目和替身验证。已有项目保持 Beads 权威直至显式迁移，不通过配置修改自动切换。

### 产品边界

- 发布前的任务草稿可以编辑；发布后固定范围、规格、验收、优先级和外部依赖。本版不提供编辑已发布任务的业务入口。
- 系统不提供拆分、撤销、重新打开、任务回退、执行中依赖修改、级联补偿或动态重规划。不实现父子任务、聚合节点、拆分状态、聚合验收或通用强制写入/整图替换命令。
- 拆分与回退是用户行为。尚未交付任务无论是否在执行，都可由用户通过 Codex 分析并人工拆分；人工拆分应保持原对外交付承诺、不让后继提前执行。已交付任务需改变交付事实时按人工回退处置，不作为普通拆分对象。
- 用户自行决定代码、PR、任务记录、依赖及运行现场的处置方式；本 Spec 不实现该流程，也不承诺自动识别、修复任意人工改动。人工处置不受产品是否提供按钮或命令的限制。
- 故障恢复、断线对账、原任务契约下的重试和人工问答继续沿用执行模型。暂停派发不代表 worker 已停止，执行失败不自动重开、释放 claim 或修改依赖。
- 只支持同项目硬前置：`A → B` 表示 A 交付后 B 才能领取，多个前置为 AND。不提供条件/OR 边、部分交付放行、跨项目依赖或接口提前满足的特殊门槛。
- 任务 DAG 只管任务之间的资格；实现/验证/审查/发布与重试仍由 WorkflowRun/Execution/StepAttempt 管理。保持既有执行槽策略，不增加并行选择器、资源锁系统或自动合并。

## N2：权威、模型与不变量

复用中心执行模型的数据库、项目身份、仓库锁、事件、命令队列及 Operation/outbox。CLI/UI 只查询或提交命令，不直接改业务表；图算法库和渲染器均不是写者。

| 记录 | 必需字段/关系 | 不变量 |
| --- | --- | --- |
| ProjectTaskAuthority | project_id、kind=beads/native、authority_version、graph_revision | 数据库为权威；配置声明必须与它一致。graph_revision 在发布、交付和导入改变图或任务读模型时递增 |
| Task | task_id、project_id、batch_id、title、type、priority、contract_id、status、claimant、membership、version、created_at | 原生 ID 不透明稳定，不由标题、文件名、Beads ID 派生；同项目 ID 唯一；membership=published/legacy_history，旧历史允许无 batch/contract |
| TaskContract | contract_id、目标 repository/remote/branch、spec_refs、plan_refs、acceptance_refs、digest | 至少一个 Spec 和验收引用；Plan 可选。每个文件引用固定仓库相对路径、完整 Git commit 和 blob 内容 SHA-256 |
| TaskDependency | project_id、predecessor_id、successor_id | 外键必须同项目、无自环/重复/环；业务 API 只在后继初次发布时添加 |
| TaskBatch | batch_id、project_id、schema_version、payload_digest、published_at、command_id | 一次发布的边界；不是可回退的整图版本；批次内局部 ID 到 task_id 的映射持久保存 |
| TaskDelivery | task_id、kind=verified_pr/imported_baseline、selected_workflow_run_id、evidence、recorded_at | 每任务一份权威交付；原生执行仅用 verified_pr；imported_baseline 仅用于 N7，缺真实历史运行时 selected_workflow_run_id 可空 |
| ExternalTaskRef | source、source_project_identity、external_id、task_id | 外部身份三元组唯一；同外部任务的多次历史运行关联一个 Task |

Task 状态仅有 open、in_progress、closed：发布为 open，领取为 in_progress，交付事务变为 closed。claimant 发布时为空，领取后保留作审计。没有 closed → open、in_progress → open 或直接关闭未领取原生任务的普通命令；导入历史基线是 N7 的单独初始化行为。

一个 Task 只允许一个权威活动 WorkflowRun，沿用执行模型的重试资格。不同尝试/候选的身份与任务分开，不能添加限制未来多个 Execution 的 `UNIQUE(workflow_run_id)`。任务重试不改 TaskContract、不重新领取同一任务；不存在领取的新任务通道与重试通道同时派发的情况。

`ready` 为派生查询：已发布、open、未领取、类型允许、全部前置有有效 TaskDelivery 且 closed。槽位、派发暂停、预检失败分别是启动资格，不伪装为依赖边或任务状态。失败、未知结果、等待决策/合并由执行模型呈现。

规格路径必须在目标仓库内，拒绝绝对路径、越界路径及跟随到仓库外的链接。发布时读取已提交 blob 核对摘要和验收章节存在；不要求理解自然语言来证明规格充分。领取前与执行端改代码前核对实际文件内容，使用 blob 字节/一致的 Git 规范化读取，不能因 Windows 换行转换误报；不要求执行 base 等于规格引用的 commit。实际规格不符则阻塞，不能静默换成最新版。所有规格与 Plan 必须与该任务同仓库。

任务范围与验收的语义充分性由用户/规格审阅负责。系统检查结构、引用、身份与依赖，不能把通过环检测描述为任务已具备正确业务前置。

## N3：草稿输入与原子发布

草稿使用 `task-batch/v1` JSON；未知字段拒绝。文件不属于业务权威，只有中心提交成功的任务可调度。

| 字段 | 语义 |
| --- | --- |
| schema_version | 固定 `task-batch/v1`，独立于 runtime 和控制协议版本 |
| project_id | 已注册目标项目，必须与配置及命令目标一致 |
| tasks[] | 非空；每项包含唯一局部 key、title、type、priority、contract 和可选 goal_ids |
| dependencies[] | 每项含 predecessor、successor；前置是 `{local_key}` 或 `{task_id}` 二选一，后继只能是本批次 `{local_key}` |

title 非空；type 限 task/bug/feature/chore 并受项目 executable_types 限制；priority 为 0–4 的整数，越小优先，省略为 2。contract 使用 N2 字段，goal_ids 仅为追踪字符串，不自动引用或解析任何规划文档。

CLI 新增独立 `task` 子命令，保留既有 `tasks` 候选快照命令的用途：

| 入口 | 中心语义 | 副作用 |
| --- | --- | --- |
| `task validate -file <json>` | ValidateTaskBatch，返回摘要、错误节点/环路径及采样版本 | 只读校验，无任务/派发写入 |
| `task publish -file <json> -request-id <id> -expected-authority-version <n>` | PublishTaskBatch，返回 batch_id、稳定 ID 映射、graph_revision 和回执 | 中心原子发布，不自动提交 start_task |
| `task show -task-id <id>` / `task list` | TaskDetail / ListTasks | 只读 |
| `task graph` / `task export` | N8 图查询/导出 | 只读业务状态；export 可写用户指定输出文件 |
| `task import-beads -file <snapshot> -dry-run` | N7 导入预演 | 只读校验，无切换 |
| `task import-beads -file <snapshot> -request-id <id> -expected-authority-version <n>` | N7 一次性导入切换 | 中心事务，不自动恢复派发 |

上述入口均接受 `-config`；发布、导入和候选启动通过已有命令服务，身份、版本、幂等键、发起者、期限和回执遵循中心命令契约。命令传入实际 payload，不让服务稍后重新读取一个可变化的客户端路径。validate 不是锁或授权，publish 必须重新校验。客户端超时按原 request_id 查询，不换 ID 重发。

payload_digest 使用规范化 JSON（对象键排序、UTF-8、无格式空白，数组顺序保留）的 SHA-256；解析拒绝重复键。命令幂等比较包含类型、项目和规范 payload。相同 ID/内容先返回已保存结果，不因成功后 authority_version 变化而重新准入；不同内容返回 idempotency_conflict。不同 request_id 是不同创建请求，系统不依据相似标题或相同任务正文自行去重。

发布前在事务外读取固定 Git 对象并核验规格，事务内确认核验输入摘要与命令一致；目标 repository/remote/branch 必须匹配该项目绑定，配置漂移时拒绝，不把同项目不同目标分支混成一张可调度图。发布事务另核验：项目为 native、authority_version 匹配；全部前置引用存在且属于 published 集合；后继仅为新任务；项目与类型有效；无自环、重复边和环。任务、契约、边、批次映射、graph_revision、事件及命令成功回执同事务提交。失败无部分任务/可领取节点；提交后回执丢失可按原命令身份恢复。

发布不改变 dispatch_mode/pause。项目已经 auto 且未暂停时，新任务可以在下一次正常轮询成为候选；CLI 成功输出必须说明这一点。新建 native 项目初始为 manual 且暂停，用户通过既有命令显式开启派发。

业务状态和控制台投影应使用新兼容版本；不把新字段塞进已有不兼容 schema 版本。N1 前置实现已占用的版本不可复用，Plan 的首步固定实际版本矩阵。

## N4：领取、执行与交付

候选按 priority、published_at、task_id 稳定排序。两名客户端选择同一任务时，领取事务最多让一个成功，其余返回 claim_conflict/stale_version；ready 缓存不构成领取许可。

领取前先获取目标主线固定 base，完成可在副作用前执行的配置、规格和能力预检；不能在数据库事务中等待网络或启动 worker。随后在中心事务中重核项目权威/任务版本、前置交付、claim 和执行槽，保存 claim、WorkflowRun、所需 Execution/Attempt、准备/派发意图、事件和命令检查点。提交后才进行相应外部工作区/启动动作，并按实际接口分步保存回执；原控制命令 start_task 的 applied 门槛仍以启动身份已持久化为准。

前置使用 verified_pr 时，领取 base 必须包含其已核验 merge commit；获取主线后进行祖先核验。平台查询已合并但目标分支不包含交付成果时不启动后继，返回 base_missing_delivery。imported_baseline 按 N7 固定的基线 commit 核验。领取后实际执行端仍需证明仓库/base 和契约一致；失败保留 claim/现场并阻塞，不释放后再从新任务路径重领。

远端路径、workspace/base 证据、未知派发与取消行为服从已有后端 Spec；本 Spec 不增加本机访问远端目录或跨后端 fallback。重试和恢复不得绕过原始固定契约、结果身份及副作用不确定性守卫。

关闭原生任务必须同时满足：

1. 中心选用了所属 WorkflowRun 的有效执行结果；该工作流固定的必需验证/审查门槛已通过，证据对应发布 revision。
2. PR 身份、平台实例、仓库、目标分支与运行绑定一致，具有真实 merged 标志、时间和完整 merge commit；closed_unmerged、unknown、仅自然语言声明或 agent 退出成功均不满足。
3. 平台核验后事务重新确认权威、task/run 版本、claim 和结果绑定，写入唯一 TaskDelivery、closed、WorkflowRun completed、事件及 graph_revision；迟到或其他尝试的结果不能替代选用结果。

原生结项不调用 bd close 或旧 completion 脚本。不允许一个 PR 被多个原生任务复用；旧历史绑定冲突在导入时报告，不覆盖。关闭重复执行返回既有交付，不重复释放/启动后继；外部查询失败不关闭任务。

后继就绪从已提交状态计算，不逐个修改后继状态。交付事务失败不释放依赖；事务已提交而进程退出、候选投影写失败时，重启以数据库恢复并重建查询。数据库事务失败回滚属于存储原子性，不是产品任务回退。

## N5：查询与错误

TaskDetail 至少返回 task_id/project_id、契约摘要/定位、状态、claim、priority、version、前置/后继 ID、dependency_ready、未满足的前置及其状态、startup_blockers、关联运行、交付证据和外部来源。无 Delivery 的异常 closed 记录不得视为 ready 前置，显示 inconsistent_delivery 并阻塞。

查询返回 schema_version=`native-tasks/v1`、project_id、authority_version、graph_revision、sampled_at。任务、边、交付和关联状态在同一数据库读取快照中采样；查询数据版本使用覆盖该快照的 database revision，不能用仅含拓扑变更的版本代替执行状态版本。列表默认每页 100、最大 500；游标固定数据库采样版本，版本已不可读时返回 cursor_invalidated，客户端刷新重读，不拼接不同版本。

错误为 code + message + 相关 ID/字段；CLI/UI 按 code 分支，不解析自然语言。除已有控制/执行错误外，本 Spec 至少提供：invalid_batch、unsupported_schema、contract_mismatch、missing_predecessor、cross_project_dependency、dependency_cycle、duplicate_dependency、immutable_task、authority_mismatch、stale_version、idempotency_conflict、task_not_ready、base_missing_delivery、inconsistent_delivery、unsupported_import、import_not_quiescent、snapshot_changed、renderer_unavailable、render_failed、export_limit_exceeded。

服务离线时可读取带采样时间的既有快照，显示过期/离线；不得据离线图直接启动。旧 beads_status 仅表示历史来源，不能改名冒充原生状态。原生 TaskDelivery 与导入的人工基线在 UI 中分别标明。

## N6：无 Beads 接入契约

配置增加 `task_source.kind=beads|native`。没有该字段的既有项目按 beads 兼容；显式 native 且没有旧任务/旧运行状态的新项目可初始化原生权威。已有状态不得因改配置初始化第二份数据库，配置与持久权威不符返回 authority_mismatch。

| 修改面 | 原生模式要求 |
| --- | --- |
| 配置/check | 不执行 bd/bv help、ready、context，不要求 Beads 目录或数据库；原生配置若仍设置旧 completion 则拒绝并解释不兼容 |
| service/控制入口 | ready、claim、详情、恢复与关闭使用同一中心任务服务；旧 Beads 查询隔离在兼容实现中 |
| workspace/worker | 核对 task_id、contract digest、run/attempt 和实际 workspace/base；清除注入给 worker 的 BEADS_DIR/BEADS_ACTOR，不从它们推断身份 |
| prompt/Schema | 原生上下文版本 `native-task/v1`；包含项目/任务/契约、claim、运行/执行/尝试、固定仓库/分支/base 和对应步骤输入 |
| 技能入口 | 遵循 Spec 02 的 minimal-v1/project_steps；不再次领取、不写中心表、不调用 bd/bv、拆分或后继解锁脚本，只返回本步骤事实 |
| 结项 | 中心查询平台证据并提交 N4 事务；不执行 Beads completion、BV 刷新或隐式通用回调 |
| 查看器/快照 | 当前事实来自 QueryService；旧外部身份保留可追踪映射，不能再为原生详情调用 bd show |

新原生项目只运行 minimal-v1，旧整段 start/finish 的 legacy-v1 仅留作 Beads 项目与历史兼容。迁移前必须把项目技能/脚本适配为已提交的 project_steps，明确输入输出版本并在隔离运行中核验。不能只加标记或提示词，宣称已消除脚本的 bd/BV 或额外结项副作用。

上述 native-task/v1 是中心到执行器的任务上下文，不修改外部 Orca 协议。worker 不连接中心 SQLite，不要求在远端安装 SteerLine 服务；使用现有结果通道回传。远端消息限制和版本化引用按 Spec 03；载荷超限必须领取前尽可能拒绝，不能通过省略关键身份或加大消息上限规避。

## N7：Beads 一次性迁移

迁移以项目为单位，中心是唯一导入写者；本版不迁移活跃 claim/WorkflowRun、待合并 PR、未完成 completion、未知 Operation、未解决决策或仍可能持有现场资源的执行。必须暂停新派发并结清这些事实，不能把“进程不在”当作已结清。外部 Beads 写入由用户协调停止，不承诺锁住其他工具。

### 固定输入

导入输入为 `beads-import/v1` JSON 快照，包含 source_project_identity（原 project/database/backend 身份）、目标 project/repository/branch、导出工具版本与时间、未完成任务及其完整前置闭包、原始状态/assignee/类型/元数据/边、Spec/Plan 映射、原 ready 集合和历史完成证据。规范摘要规则沿用 N3。快照由只读导出器经已验证的 Beads CLI JSON 生成，不读取内部数据库文件；具体 CLI 版本/命令与样本必须在 BEADS-EXIT 中固定并测试，未知关系类型保留后拒绝，不静默丢弃。

所有剩余未完成任务须 open 且未分配，特殊 blocked/in_progress 或外部 assignee 需人工整理；保留的已完成前置允许保存历史领取者。epic/aggregate、split 元数据、动态后继规则、跨项目边、循环、缺失闭包或契约引用不全均返回 unsupported_import，报告具体对象。用户人工整理后生成新快照，不由导入器自动扁平化。

每个历史 closed 前置必须有以下之一：可追踪并重新核验的 PR 交付依据，以其 merge commit 作为基线；或用户明确提供的 `accepted_baseline`（task external ID、目标仓库/分支、包含所需成果的完整 baseline_commit、验收引用、确认者、确认时间和说明）。基线在发布的目标主线上核验可达，不伪造历史 WorkflowRun/验证/审查成功；写入 imported_baseline 并区分 evidence 来源为平台核验或人工确认。仅一个 closed 字段不释放依赖。

### 预演与切换

1. dry-run 核对源/目标身份、闭包、原生约束、规格与历史证据，以及项目接入能力；只生成包含 snapshot_digest、authority_version、预期图/ready、原 ready 差异及阻塞项的报告，不改任务与权威。
2. 正式导入通过 `-report <report.json>` 携带预演报告和摘要；存在 ready 差异时还必须指定 `-accept-differences`，确认绑定到该报告摘要。中心重新读取并比较源的任务/依赖/状态摘要、目标权威版本及执行静默条件；不同则 snapshot_changed/stale_version，要求重新预演。前后两次一致检查不宣称能阻止外部并发写，操作期间仍由用户保证源冻结。客户端声明的报告不能替代中心重算，含任何阻塞项的报告即使确认也拒绝。
3. 规格/平台/基线等外部读取完成后，在单个事务再次核验中心状态，导入任务、契约、全部边、交付基线、来源映射、迁移摘要与事件，并把权威切为 native、递增版本；暂停状态保留。命令参数中的 expected-authority-version 与该事务 CAS 一致。
4. 已由 Spec 02 建立的同来源 Task 必须复用其内部 ID，既有运行/PR/消息引用不重建。未建立者分配新 ID；冲突、历史 PR 重复绑定或身份不符使整个导入失败。历史非闭包任务保留 legacy_history 只读来源标记，不成为原生候选，不为它们虚构交付。
5. 事务失败保持旧权威；提交后响应丢失按原 request_id/snapshot_digest 查回原映射，不再导入。投影/配置文件更新失败不回滚数据库，查询显示实际 native 权威；旧配置不能启动 Beads 路径。
6. 切换后把配置声明更新为 native，再通过既有命令显式恢复派发；该配置更新是接入步骤，不是第二权威。此后正常读写不得访问 Beads；旧快照供追踪，不双写、不自动切回。

无需新建原生项目执行此迁移。迁移只能完成已符合约束的项目；特殊业务回调或额外交付副作用仍未适配时，拒绝切换，不通过丢弃回调使迁移“成功”。切换后的人工拆分/回退依 N1，由用户承担，不增加产品恢复工作流。

## N8：任务图与导出

图由 QueryService 返回 native-task-graph/v1，包含 N5 的采样元数据、nodes、edges、boundary_nodes、truncated 和过滤条件。节点至少展示稳定 ID、标题、状态、dependency_ready 与 blocker；边统一前置 → 后继。交付来源和规格/运行定位可在详情查询，不把所有日志装入节点。

`task graph` 支持 project、status 和以 task_id 为中心的 upstream/downstream/both、depth（默认 1、1–10）。默认最多 500 个完整节点，显式上限最大 1000；边只来自同一采样快照。每个显示节点的未显示前置/后继必须有边界 ID 或明确计数，选中详情可分页追踪；truncated 不得被标成完整图。

`task export -format json|dot|svg -output <path>` 接受同样筛选。JSON 是查询快照格式，不是可直接再次发布的 task-batch/v1；不把已发布 task_id 导出后用于重复建图。无筛选的完整导出超过上限返回 export_limit_exceeded，不悄悄裁剪。带筛选的导出保留边界与采样信息。

DOT 为必需输出；SVG 通过可选 Graphviz `dot` 生成。渲染工具缺失返回 renderer_unavailable，不影响发布、领取、TUI 或 DOT/JSON。可配置原生可执行文件路径，通过固定参数数组和 stdin 传 DOT；默认超时 30 秒、最大 SVG 20 MiB，超限/失败保留原输出文件，不影响业务状态。输出写到同目录临时文件后原子发布；已有文件无显式 overwrite 参数时拒绝。

节点标题、ID 等按纯文本转义，不解释为 DOT 语法、HTML 标签、终端控制序列或外部图片/脚本 URL。SVG 不嵌入任务提供的任意链接或资源；首版通过导出文件查看，不引入 Web 服务或浏览器内控制面。TUI 经中心查询展示前置、后继、阻塞原因及规格/交付定位，刷新保留 task_id 选择，不直接读 Beads。

图算法采用 Go 内部窄接口封装环检测、稳定遍历和拓扑顺序；优先接入 [dominikbraun/graph](https://github.com/dominikbraun/graph)，选择发布版本并锁定 go.mod/go.sum，在适配层验证其错误/排序语义。库的内存存储不代替 SQLite，库类型不进入公共 DTO。布局采用 [Graphviz dot](https://graphviz.org/docs/layouts/dot/)，渲染版本记入验证证据；渲染顺序不决定任务调度。

必须覆盖 100/500/1000 节点的链、分支汇合及不连通图，中文与长标题，以及超时、缺渲染器和超限；1000 节点的渲染须在上述预算内完成或返回明确失败，不能阻塞服务调度循环。交互式 Web 图、React Flow/Dagre/ELK.js 选型不属于本阶段。

## N9：恢复、版本与职责交接

- TASK-CORE 负责与 Spec 02 合并 Task 身份、权威和 schema，不能建立平行 TaskStore。旧 runtime 导入仍由 Spec 02 负责；本 Spec 在其上处理原生契约、依赖和 Beads 权威切换。
- TASK-DAG 负责共同发布/领取/交付和图查询语义；BEADS-EXIT 扩展接入与导入，不另建 ready/close 规则。DAG-VIEW 只消费共同查询，不修改命令或执行业务状态。
- Spec 02 中“任务提供方回读”对 beads 项目继续成立；native 项目以 N2/N4 的事务为准。任务源与工作流版本是两个独立轴，不能根据 local/Orca 或 UI 入口推断源。
- Spec 03 先落地时继续已有中心 Beads 兼容路径；native 模式按 N6 接入相同远程步骤上下文，结项走 N4。后落地的一方扩展已有公共 DTO，不能另造执行/会话模型。真实远端能力仍受 Spec 03 门槛约束。
- 数据库迁移分配实际未使用的 schema 版本；runtime/候选投影改为可重建读模型，记录 database revision 和生成错误。原生事实不依赖投影可写成功；旧读者拒绝不兼容版本，不能误读 native 为 beads。
- 幂等成功命令、任务身份、契约、交付、PR 绑定、来源映射与历史运行均持久保留。文档或展示删除已完成条目不删除这些记录。本 Spec 不实现业务历史清理。

## N10：验收矩阵

| 编号 | 阶段 | 场景与通过条件 |
| --- | --- | --- |
| NT01 | CORE | 无外部 ID 的 Task 可存取；project 隔离；多历史运行关联不丢；Task/Execution 身份分离 |
| NT02 | CORE | 原生/Beads 权威持久化；改配置不能绕过迁移；旧投影与历史来源语义正确 |
| NT03 | CORE | Spec/Plan revision、摘要、验收引用核对；路径越界、缺文件、摘要不符拒绝；CRLF checkout 不误报 |
| NT04 | DAG | A → B → C、分支汇合、不连通任务就绪正确；部分前置完成不放行；稳定优先级顺序 |
| NT05 | DAG | 自环、循环、悬空、重复、跨项目边、旧后继新增前置和已发布契约编辑均拒绝，无局部写入 |
| NT06 | DAG | 同 request_id 重发只创建一次；异内容拒绝；事务写入任一步失败全部回滚，提交后丢回执可查回 |
| NT07 | DAG | 两客户端竞争只一个领取；原 ready 过期、暂停、槽被占、前置未交付分别正确拒绝 |
| NT08 | DAG | 领取提交前后、准备/启动响应前后重启；无重复 claim/worker，不确定动作保留原身份与资源 |
| NT09 | DAG | PR 未合并、closed_unmerged、错仓库/主线/身份、无验证证据不关闭；真实交付事务关闭且释放后继 |
| NT10 | DAG | 交付事务失败/成功但投影失败/响应丢失时后继资格准确；重复关闭、迟到结果和重复 PR 无二次释放 |
| NT11 | DAG | 获取的 base 缺前置 merge/baseline commit 或执行端规格不符时不开发；重试不改变固定任务契约 |
| NT12 | EXIT | 无 bd/bv、无 Beads 环境的新项目通过 check、创建、启动、验证/发布、真实合并核验和后继就绪 |
| NT13 | EXIT | 旧 start/finish、completion 或隐含 bd/BV 脚本未适配则预检失败；原生 worker 不再收到 Beads 环境/路径 |
| NT14 | EXIT | 迁移正常闭包/来源/历史基线，复用已存在 Task ID；ready 差异明确；不伪造历史步骤成功 |
| NT15 | EXIT | 活跃/未知执行、待合并/回调、外部 assignee、聚合/动态关系、未知 closed、源变更、PR 冲突拒绝切换 |
| NT16 | EXIT | 导入事务失败无切换；提交后丢回执/配置写失败/重启恢复同一映射；切换后 normal path 无 Beads 调用 |
| NT17 | EXIT | local 与已支持本机 Orca 的原生路径均通过；已启用 supervised 时追加真实远端回归，不扩大版本/取消能力 |
| NT18 | VIEW | 同一读取快照图/列表一致；ID、方向、交付来源、边界、离线时间正确；过期游标不得拼接 |
| NT19 | VIEW | 中文/长标题、100/500/1000 节点、布局失败/超限/缺工具可解释，服务调度不被渲染阻塞 |
| NT20 | VIEW | 文本转义、无脚本/外链注入、输出不意外覆盖；UI/CLI 无拆分、回退或编辑已发布依赖入口 |
| NT21 | ALL | 既有 Beads 项目在切换前、watch/answer/retry/accept-result、会话与控制生命周期回归通过；恢复无第二权威 |

最终演示使用隔离项目 A → B：无 Beads 安装，B 最初阻塞；A 按固定契约执行、真实 PR 经用户合并后中心核验交付，B 就绪；中途重启中心仍保留身份及正确资格。不得自动合并 PR。按阶段保存检查命令、版本、输入摘要、回执与未执行项，真实证据不足的验收不得标为完成。
