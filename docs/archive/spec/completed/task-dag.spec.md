# Spec：固定依赖 DAG 调度

任务 ID：8；任务名称：固定依赖调度；目标：`TASK-DAG`。状态：D1 中心范围已实现并通过验收，[PR #31](https://github.com/erranliu/SteerLine/pull/31) 待合入；生产动作按本 Spec 的边界由 BEADS-EXIT 验收。配套 [Plan](task-dag.plan.md) 与[验证记录](../../../validation/task-dag/README.md)。本文件只定义该任务，由一个独立实现 PR 交付。

## D1：范围与前置

前置：[原生任务模型 Spec](task-core.spec.md) 已完整验收并合入。引用 [原生任务共同约束](../../../spec/native-task-constraints.md) 全部条款，使用前置 C2/C3 的身份、存储和基础查询，不重建 TaskStore。

本任务交付输入校验、批次原子创建并入 DAG、显式丢弃、硬依赖/ready、事务领取、交付释放、恢复及关系读模型。不交付 TUI 关系页或渲染导出。使用隔离项目与替身验证业务规则；生产 native worker 接入已由 [Beads 退出 Spec](beads-exit.spec.md) 实现。本任务保存固定外部动作身份、意图与清理待办；替身结果不能证明真实 worker 启停或外部清理平台已经验收。

依赖只有同项目 AND 硬前置；创建后固定边和契约，不提供改边入口。PR 合并前可丢弃任务，即使它已入 DAG 或开始执行；丢弃不删除依赖边，不等于 split/rollback/reopen 或自动修图。本任务不增加并行调度。

## D2：输入与原子创建

`task-batch/v1` JSON 仅是创建命令的输入；未知字段拒绝。中心在同一事务中创建任务、固定契约和依赖并使其进入 DAG，没有“先创建任务、以后再发布”的状态。未提交的文件不是业务权威。

| 字段 | 语义 |
| --- | --- |
| schema_version | 固定 `task-batch/v1`，独立于 runtime 和控制协议版本 |
| project_id | 已注册目标项目，必须与配置及命令目标一致 |
| tasks[] | 非空；每项包含唯一局部 key、预先分配的非负整数 task_id、title、type、priority、contract 和可选 goal_ids |
| dependencies[] | 每项含 predecessor、successor；前置是 `{local_key}` 或 `{task_id}` 二选一，后继只能是本批次 `{local_key}` |

title 非空；type 限 task/bug/feature/chore 并受项目 executable_types 限制；priority 为 0–4 的整数，越小优先，省略为 2。contract 使用 [原生任务模型 Spec C2](task-core.spec.md) 的单 Spec/单 Plan 字段，goal_ids 仅为追踪字符串，不承载 task_id 的唯一身份，也不自动引用或解析任何规划文档。

`task_id` 是该任务在规划阶段已取得的对外身份，创建事务原值保存，不另生成 ID。同批次重复数字、同一规范仓库身份下已创建的数字 task_id 冲突均拒绝，即使项目配置不同；幂等重放原创建请求返回原任务。数字不参与优先级或依赖判定。中心不读取规划文档或代为分配 ID；缺少 task_id 的批次无效。

在原生任务模型已提供的 `task` 子命令下新增以下入口，保留既有 `tasks` 候选快照命令的用途：

| 入口 | 中心语义 | 副作用 |
| --- | --- | --- |
| `task validate -file <json>` | ValidateTaskBatch，返回摘要、错误节点/环路径及采样版本 | 只读校验，无任务/派发写入 |
| `task create -file <json> -request-id <id> -expected-authority-version <n>` | CreateTaskBatch，返回 batch_id、局部 key 到原输入 task_id 的确认映射、graph_revision 和回执 | 中心原子创建并入 DAG，不自动提交 start_task |

上述入口均接受 `-config`；创建和候选领取通过已有中心命令服务，身份、版本、幂等键、发起者、期限和回执遵循中心命令契约。命令传入实际 payload，不让服务稍后重新读取一个可变化的客户端路径。validate 不是锁或授权，create 必须重新校验。客户端超时按原 request_id 查询，不换 ID 重发。

规划器以同一次用户操作形成任务身份、已提交的 Spec/Plan 和 roadmap 关系，再以该固定 Git commit 与同一 task_id 调用 CreateTaskBatch。Git 提交与中心 SQLite 不构成跨系统原子事务；只有两侧均确认后才报告任务创建完成。任一侧失败或回执不明时保留原 ID、commit 和 request_id 对账，不另分配 ID，也不暴露“先建后发布”的任务状态。

payload_digest 复用共同约束及前置原生任务模型实现的规范化规则。命令幂等比较包含类型、项目和规范 payload。相同 ID/内容先返回已保存结果，不因成功后 authority_version 变化而重新准入；不同内容返回 idempotency_conflict。不同 request_id 是不同创建请求，系统不依据相似标题或相同任务正文自行去重。

创建前在事务外读取固定 Git 对象并核验规格，事务内确认核验输入摘要与命令一致；目标 repository/remote/branch 必须匹配该项目绑定，配置漂移时拒绝，不把同项目不同目标分支混成一张可调度图。创建事务另核验：项目为 native、authority_version 匹配；任务 ID 未与已有/同批次任务冲突；全部前置引用已存在于图中且未丢弃；后继仅为本次新任务；项目与类型有效；无自环、重复边和环。任务、契约、边、局部 key 到原 task_id 的映射、graph_revision、事件及命令成功回执同事务提交。失败无部分任务/可领取节点；提交后回执丢失可按原命令身份恢复。

创建不改变 dispatch_mode/pause。项目已经 auto 且未暂停时，新任务可以在下一次正常轮询成为候选；CLI 成功输出必须说明这一点。新建 native 项目初始为 manual 且暂停，用户通过既有命令显式开启派发。

业务状态和控制台投影使用本轮确定的格式；不支持旧客户端或旧数据库格式。当前格式的任务和图版本仍用于并发控制与查询一致性。

## D3：领取、执行与交付

候选按 priority、created_at、task_id 稳定排序。两名客户端选择同一任务时，领取事务最多让一个成功，其余返回 claim_conflict/stale_version；ready 缓存不构成领取许可。

领取前先获取目标主线固定 base，完成可在副作用前执行的配置、规格和能力预检；不能在数据库事务中等待网络或启动 worker。随后在中心事务中重核项目权威/任务版本、未丢弃状态、前置交付、claim 和执行槽，保存 claim、WorkflowRun、所需 Execution/Attempt、准备/派发意图、事件和命令检查点。本任务的 claim 回执只表示预留，不报告 worker 已启动；BEADS-EXIT 在提交后按固定身份执行外部工作区/启动动作并分步保存回执。原控制命令 start_task 的 applied 门槛仍以启动身份已持久化为准。

领取 base 必须包含全部前置已核验 PR 的 merge commit；获取主线后进行祖先核验。平台查询已合并但目标分支不包含交付成果时不启动后继，返回 base_missing_delivery。领取后实际执行端仍需证明仓库/base 和契约一致；BEADS-EXIT 验收这一执行端门槛。失败保留 claim/现场并阻塞，不释放后再从新任务路径重领。

远端路径、workspace/base 证据、未知派发与取消行为服从已有后端 Spec；本 Spec 不增加本机访问远端目录或跨后端 fallback。重试和恢复不得绕过原始固定契约、结果身份及副作用不确定性守卫。

关闭原生任务必须同时满足：

1. 中心选用了所属 WorkflowRun 的有效执行结果；该工作流固定的必需验证/审查门槛已通过，证据对应发布 revision。
2. PR 身份、平台实例、仓库、目标分支与运行绑定一致，具有真实 merged 标志、时间和完整 merge commit；closed_unmerged、unknown、仅自然语言声明或 agent 退出成功均不满足。
3. 平台核验后事务重新确认权威、task/run 版本、claim、未丢弃状态和结果绑定，写入唯一 TaskDelivery、closed、WorkflowRun completed、事件及 graph_revision；迟到或其他尝试的结果不能替代选用结果。

原生结项不调用 bd close 或旧 completion 脚本。不允许一个 PR 被多个原生任务复用；重复绑定直接拒绝。关闭重复执行返回既有交付，不重复释放/启动后继；外部查询失败不关闭任务。

后继就绪从已提交状态计算，不逐个修改后继状态。交付事务失败不释放依赖；事务已提交而进程退出、候选投影写失败时，重启以数据库恢复并重建查询。数据库事务失败回滚属于存储原子性，不是产品任务回退。

### 丢弃未合并任务

`task discard -task-id <id> -request-id <id> -expected-version <n> -reason <text>` 对本次创建的原生任务提交有身份和幂等键的丢弃意图。中心绑定该任务的精确 PR、运行和版本，向平台核验 PR 尚未合并；已合并则拒绝，查询失败或合并状态未知则保留待核实意图，不宣告丢弃。无 PR 时也检查已知发布动作及不确定回执，不能把“尚未绑定 URL”当成未产生 PR 的证明。不能按标题匹配别的 PR。

核验后中心事务重查任务版本、PR 绑定与交付状态，将 open 或 in_progress 改为 discarded，保存原因、操作者、时间、请求结果和 graph_revision。丢弃不删除任务、契约、边、运行、PR 或身份记录，不生成 TaskDelivery，也不释放任何后继；其他任务若依赖它，查询返回 `discarded_predecessor` 并继续阻塞。已丢弃任务不再被领取、重试、派发后续步骤、发布新 PR 或正常结项；迟到结果只留作历史。

丢弃资格不受执行是否已开始影响。本任务在原执行仍活跃或停止结果未知时保留 Execution/Attempt 身份、资源占用和精确 `cleanup_pending`，不把待办伪称为已停止或已清理；远程取消不可用时明确要求人工处理。BEADS-EXIT 接线后须按本机已验证的 Stop 能力请求停止，并对固定身份的开放 PR 和工作区分别清理、保存回执；未知结果继续占用资源。丢弃后若通过固定 PR 绑定核实外部合并，中心幂等记录交付冲突并要求人工核实，不把 discarded 自动改成 closed 或给后继放行。相同请求幂等返回原结果，不能通过另一请求复用该 task_id。

## D4：依赖查询与算法

扩展前置 native-tasks/v1 查询：TaskDetail 返回前置/后继 ID、dependency_ready、未满足前置及状态、startup_blockers、丢弃原因和清理待办。全部字段从同一数据库读取快照计算，ready 与执行槽/暂停/能力阻塞分开；丢弃前置返回 `discarded_predecessor`，异常 closed 而无 Delivery 返回 inconsistent_delivery 并阻塞，不把缺证据当成完成。

在前置 C3 的版本/分页规则上提供关系读模型，包含节点、边、交付来源和阻塞原因。基础查询版本与 database_revision 保持一致；后续 [关系视图 Spec](dag-view.spec.md) 在其上实现图筛选、独立导出协议和 UI，不复制 ready/close 规则。

新增错误至少包括 invalid_batch、missing_predecessor、cross_project_dependency、dependency_cycle、duplicate_dependency、immutable_task、idempotency_conflict、task_not_ready、task_discarded、discarded_predecessor、merge_state_unknown、already_merged、base_missing_delivery、inconsistent_delivery，沿用前置的 schema/版本/权威与既有控制错误。

在中心服务内实现固定 DAG 所需的环检测和稳定遍历；以已提交的 SQLite 任务与依赖为权威，不引入额外图存储或第三方图框架。调度顺序由任务规则决定，不由图展示位置决定。

## D5：恢复与交接

创建、丢弃、领取、交付的状态/事件/意图按共同约束持久化，崩溃后重算 ready，外部动作不确定时保留原 Operation/attempt 与占用。丢弃后的清理待办、PR 状态和迟到结果仍可对账，不增加通用状态修复入口。

向 [Beads 退出 Spec](beads-exit.spec.md) 交接稳定创建/丢弃/领取/交付服务与契约上下文输入，向 [关系视图 Spec](dag-view.spec.md) 交接只读关系模型和阻塞语义；两者均复用本任务的业务算法。规格足够执行的业务判断不由环检测替代。

## 验收

| 编号 | 场景与通过条件 |
| --- | --- |
| NT04 | A → B → C、分支汇合、不连通任务就绪正确；部分前置完成不放行；稳定优先级顺序 |
| NT05 | 自环、循环、悬空、重复、跨项目边、重复或已占用 task_id、旧后继新增前置和已创建契约编辑均拒绝，无局部写入 |
| NT06 | 同 request_id 重发只创建一次；异内容拒绝；事务写入任一步失败全部回滚，提交后丢回执可查回；规划 Git 提交/中心创建部分失败沿用原 ID 与请求对账，不出现独立待发布状态或第二 Task |
| NT07 | 两客户端竞争只一个领取；原 ready 过期、暂停、槽被占、前置未交付分别正确拒绝 |
| NT08 | 领取事务提交前后与回执丢失后重启，同请求只返回原 claim、WorkflowRun、Execution、Attempt 和外部意图；不把预留报告为 worker 已启动，不确定动作保留原身份与资源。实际准备/启动响应及 worker 去重由 BEADS-EXIT NT16/NT17 验收 |
| NT09 | PR 未合并、closed_unmerged、错仓库/主线/身份、无验证证据不关闭；真实交付事务关闭且释放后继 |
| NT10 | 交付事务失败/成功但投影失败/响应丢失时后继资格准确；重复关闭、迟到结果和重复 PR 无二次释放 |
| NT11 | 获取的 base 缺前置 merge/baseline commit 或固定规格在领取前不符时不预留；原契约摘要/base 随运行固定，中心结果门槛拒绝变化的契约证据。实际执行端工作区/规格核验和原契约下重试由 BEADS-EXIT NT12/NT13 验收 |
| NT22-DAG | 未领取、运行中、等待 PR 的任务在 PR 未合并时均可丢弃；已合并拒绝，合并状态未知待核实；丢弃不释放后继、不复用 ID，活跃/远端 unknown 保留资源和精确清理待办；迟到结果及核实的丢弃后合并只记录冲突，不伪造正常交付。外部停止/PR/工作区清理回执由 BEADS-EXIT NT22-EXIT 验收 |
| NT21-DAG | 既有 start_task、自动派发、native retry 与控制守卫回归通过；原生中心查询和状态转换只有一个权威实现。生产接线由 BEADS-EXIT NT21-EXIT 覆盖；退役的 whole-run accept-result 不属于当前契约 |

本任务的全部验收必须在一个独立实现 PR 内完成；配套 Plan 只负责本任务。证据不足不得标为完成。
