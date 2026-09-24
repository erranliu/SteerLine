# Spec：固定依赖 DAG 调度

任务：`TASK-DAG`。状态：待实施。配套 [Plan](task-dag.plan.md)。本文件只定义该任务，由一个独立实现 PR 交付。

## D1：范围与前置

前置：[原生任务模型 Spec](task-core.spec.md) 已完整验收并合入。引用 [原生任务共同约束](native-task-constraints.md) 全部条款，使用前置 C2/C3 的身份、存储和基础查询，不重建 TaskStore。

本任务交付草稿校验、批次原子发布、硬依赖/ready、事务领取、交付释放、恢复及关系读模型。不交付生产无 Beads 的 worker/技能接入、项目迁移、TUI 关系页或渲染导出。使用隔离项目与替身验证业务规则；生产原生接入仍等待 [Beads 退出 Spec](beads-exit.spec.md)。

依赖只有同项目 AND 硬前置；发布后禁止变更。没有 split/rollback/reopen、父子聚合或自动修图入口。重试与多候选模型保留既有职责，不在本任务增加并行调度。

## D2：草稿与发布

草稿使用 `task-batch/v1` JSON；未知字段拒绝。文件不属于业务权威，只有中心提交成功的任务可调度。

| 字段 | 语义 |
| --- | --- |
| schema_version | 固定 `task-batch/v1`，独立于 runtime 和控制协议版本 |
| project_id | 已注册目标项目，必须与配置及命令目标一致 |
| tasks[] | 非空；每项包含唯一局部 key、title、type、priority、contract 和可选 goal_ids |
| dependencies[] | 每项含 predecessor、successor；前置是 `{local_key}` 或 `{task_id}` 二选一，后继只能是本批次 `{local_key}` |

title 非空；type 限 task/bug/feature/chore 并受项目 executable_types 限制；priority 为 0–4 的整数，越小优先，省略为 2。contract 使用 [原生任务模型 Spec C2](task-core.spec.md) 的单 Spec/可选 Plan 字段，goal_ids 仅为追踪字符串，不自动引用或解析任何规划文档。

在原生任务模型已提供的 `task` 子命令下新增以下入口，保留既有 `tasks` 候选快照命令的用途：

| 入口 | 中心语义 | 副作用 |
| --- | --- | --- |
| `task validate -file <json>` | ValidateTaskBatch，返回摘要、错误节点/环路径及采样版本 | 只读校验，无任务/派发写入 |
| `task publish -file <json> -request-id <id> -expected-authority-version <n>` | PublishTaskBatch，返回 batch_id、稳定 ID 映射、graph_revision 和回执 | 中心原子发布，不自动提交 start_task |

上述入口均接受 `-config`；发布和候选启动通过已有命令服务，身份、版本、幂等键、发起者、期限和回执遵循中心命令契约。命令传入实际 payload，不让服务稍后重新读取一个可变化的客户端路径。validate 不是锁或授权，publish 必须重新校验。客户端超时按原 request_id 查询，不换 ID 重发。

payload_digest 复用共同约束及前置原生任务模型实现的规范化规则。命令幂等比较包含类型、项目和规范 payload。相同 ID/内容先返回已保存结果，不因成功后 authority_version 变化而重新准入；不同内容返回 idempotency_conflict。不同 request_id 是不同创建请求，系统不依据相似标题或相同任务正文自行去重。

发布前在事务外读取固定 Git 对象并核验规格，事务内确认核验输入摘要与命令一致；目标 repository/remote/branch 必须匹配该项目绑定，配置漂移时拒绝，不把同项目不同目标分支混成一张可调度图。发布事务另核验：项目为 native、authority_version 匹配；全部前置引用存在且属于 published 集合；后继仅为新任务；项目与类型有效；无自环、重复边和环。任务、契约、边、批次映射、graph_revision、事件及命令成功回执同事务提交。失败无部分任务/可领取节点；提交后回执丢失可按原命令身份恢复。

发布不改变 dispatch_mode/pause。项目已经 auto 且未暂停时，新任务可以在下一次正常轮询成为候选；CLI 成功输出必须说明这一点。新建 native 项目初始为 manual 且暂停，用户通过既有命令显式开启派发。

业务状态和控制台投影应使用新兼容版本；不把新字段塞进已有不兼容 schema 版本。前置实现已占用的版本不可复用，配套 Plan 首步固定实际版本矩阵。

## D3：领取、执行与交付

候选按 priority、published_at、task_id 稳定排序。两名客户端选择同一任务时，领取事务最多让一个成功，其余返回 claim_conflict/stale_version；ready 缓存不构成领取许可。

领取前先获取目标主线固定 base，完成可在副作用前执行的配置、规格和能力预检；不能在数据库事务中等待网络或启动 worker。随后在中心事务中重核项目权威/任务版本、前置交付、claim 和执行槽，保存 claim、WorkflowRun、所需 Execution/Attempt、准备/派发意图、事件和命令检查点。提交后才进行相应外部工作区/启动动作，并按实际接口分步保存回执；原控制命令 start_task 的 applied 门槛仍以启动身份已持久化为准。

前置使用 verified_pr 时，领取 base 必须包含其已核验 merge commit；获取主线后进行祖先核验。平台查询已合并但目标分支不包含交付成果时不启动后继，返回 base_missing_delivery。imported_baseline 按 [Beads 退出 Spec B3](beads-exit.spec.md) 固定的基线 commit 核验。领取后实际执行端仍需证明仓库/base 和契约一致；失败保留 claim/现场并阻塞，不释放后再从新任务路径重领。

远端路径、workspace/base 证据、未知派发与取消行为服从已有后端 Spec；本 Spec 不增加本机访问远端目录或跨后端 fallback。重试和恢复不得绕过原始固定契约、结果身份及副作用不确定性守卫。

关闭原生任务必须同时满足：

1. 中心选用了所属 WorkflowRun 的有效执行结果；该工作流固定的必需验证/审查门槛已通过，证据对应发布 revision。
2. PR 身份、平台实例、仓库、目标分支与运行绑定一致，具有真实 merged 标志、时间和完整 merge commit；closed_unmerged、unknown、仅自然语言声明或 agent 退出成功均不满足。
3. 平台核验后事务重新确认权威、task/run 版本、claim 和结果绑定，写入唯一 TaskDelivery、closed、WorkflowRun completed、事件及 graph_revision；迟到或其他尝试的结果不能替代选用结果。

原生结项不调用 bd close 或旧 completion 脚本。不允许一个 PR 被多个原生任务复用；旧历史绑定冲突在导入时报告，不覆盖。关闭重复执行返回既有交付，不重复释放/启动后继；外部查询失败不关闭任务。

后继就绪从已提交状态计算，不逐个修改后继状态。交付事务失败不释放依赖；事务已提交而进程退出、候选投影写失败时，重启以数据库恢复并重建查询。数据库事务失败回滚属于存储原子性，不是产品任务回退。

## D4：依赖查询与算法

扩展前置 native-tasks/v1 查询：TaskDetail 返回前置/后继 ID、dependency_ready、未满足前置及状态、startup_blockers。全部字段从同一数据库读取快照计算，ready 与执行槽/暂停/能力阻塞分开；异常 closed 而无 Delivery 返回 inconsistent_delivery 并阻塞，不把缺证据当成完成。

在前置 C3 的版本/分页规则上提供关系读模型，包含节点、边、交付来源和阻塞原因。基础查询版本与 database_revision 保持一致；后续 [关系视图 Spec](dag-view.spec.md) 在其上实现图筛选、独立导出协议和 UI，不复制 ready/close 规则。

新增错误至少包括 invalid_batch、missing_predecessor、cross_project_dependency、dependency_cycle、duplicate_dependency、immutable_task、idempotency_conflict、task_not_ready、base_missing_delivery、inconsistent_delivery，沿用前置的 schema/版本/权威与既有控制错误。

图算法采用 Go 内部窄接口封装环检测、稳定遍历和拓扑顺序；优先接入 [dominikbraun/graph](https://github.com/dominikbraun/graph)，选定发布版本并锁定 go.mod/go.sum，验证其错误与排序语义。库的内存存储不代替 SQLite，库类型不进入公共 DTO，调度顺序不由可视化位置决定。

## D5：恢复与交接

发布、领取、交付的状态/事件/意图按共同约束持久化，崩溃后重算 ready，外部动作不确定时保留原 Operation/attempt 与占用。不增加通用状态修复入口。

向 [Beads 退出 Spec](beads-exit.spec.md) 交接稳定发布/领取/交付服务与契约上下文输入，向 [关系视图 Spec](dag-view.spec.md) 交接只读关系模型和阻塞语义；两者均复用本任务的业务算法。规格足够执行的业务判断不由环检测替代。

## 验收

| 编号 | 场景与通过条件 |
| --- | --- |
| NT04 | A → B → C、分支汇合、不连通任务就绪正确；部分前置完成不放行；稳定优先级顺序 |
| NT05 | 自环、循环、悬空、重复、跨项目边、旧后继新增前置和已发布契约编辑均拒绝，无局部写入 |
| NT06 | 同 request_id 重发只创建一次；异内容拒绝；事务写入任一步失败全部回滚，提交后丢回执可查回 |
| NT07 | 两客户端竞争只一个领取；原 ready 过期、暂停、槽被占、前置未交付分别正确拒绝 |
| NT08 | 领取提交前后、准备/启动响应前后重启；无重复 claim/worker，不确定动作保留原身份与资源 |
| NT09 | PR 未合并、closed_unmerged、错仓库/主线/身份、无验证证据不关闭；真实交付事务关闭且释放后继 |
| NT10 | 交付事务失败/成功但投影失败/响应丢失时后继资格准确；重复关闭、迟到结果和重复 PR 无二次释放 |
| NT11 | 获取的 base 缺前置 merge/baseline commit 或执行端规格不符时不开发；重试不改变固定任务契约 |
| NT21-DAG | 既有 start_task、自动派发、retry/accept-result、Beads 兼容运行与控制守卫回归通过；原生查询和状态转换只有一个权威实现。 |

本任务的全部验收必须在一个独立实现 PR 内完成；配套 Plan 只负责本任务。证据不足不得标为完成。
