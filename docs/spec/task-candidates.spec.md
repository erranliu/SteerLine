# Spec：多候选模型与准入

任务 ID：26；任务名称：多候选模型与准入；目标：`PHASE1-CANDIDATES`。配套 [Plan](task-candidates.plan.md)。状态：待实现与验收。一个独立实现 PR 交付。

## 范围与前置

无未完成硬前置。复用已交付的[不同任务人工并行](manual-parallel-start.spec.md)、[固定 DAG](task-dag.spec.md)和[共同约束](phase1-delivery-constraints.md)。本任务交付同 Task 的多个独立 WorkflowRun、启动准入、精确命令目标、完整读模型与恢复；PR 胜出和落选结算由 #27 交付。

结构重构安排拟通过中心追加 #34 为开工前置，消费[执行上下文](execution-context.spec.md)和其前置 #33 的纯规则；原已创建契约没有该边。活动运行须先满足原生安全重置条件，补边回执取得前不把拟定顺序当作已生效。本任务的候选策略、schema 转换与验收范围保持。

Task 创建契约增加 execution_policy，取值 `single` / `multiple`，省略时为 single；新契约摘要包含规范化后的策略。既有固定契约原字节/摘要保留，缺字段按原 single 事实解释，不把原 Task 升成 multiple。创建后不修改策略。multiple 不设置业务数量上限；后端启动失败按该请求单独结算，不推断可用容量或引入配置数量上限。人工显式启动候选；自动派发仍按既有单活动执行策略只领取未领取 Task 的首个候选，不自动生成竞争候选。

摘要模式固定为 legacy-v1 和 candidates-v2，由 task_contracts 的持久元数据区分。schema 5 转换时原记录标为 legacy-v1，保留原 payload 字节与 digest；校验仍按原 TaskContract 字段集合序列化、将 digest 置为空字符串并做既有 CanonicalJSON/SHA-256，保留原 null/空数组语义，缺策略仅在业务读模型中解释为 single。新创建记录一律 candidates-v2，将 execution_policy 显式规范为 single/multiple 后参与相同规范摘要算法；未知字段拒绝。缺失/未知模式、模式与记录形状不符时拒绝业务操作，不按“哪个摘要能匹配”回退。转换前先用 legacy-v1 核验全部原契约，转换后分别按存储模式校验；新请求不能指定 legacy-v1 来绕过策略约束。

## 身份和准入

候选 ID 直接使用 workflow_run_id，不另建竞争的 Candidate 权威对象。每个候选固定自己的 Execution、StepAttempt、执行配置、agent/目标、分支、worktree、Operation、会话与 Decision。重试继续属于原候选；新增候选是新请求、新 WorkflowRun，不复用其外部执行。一个候选等待、失败或 unknown 时，同任务其他候选仍可被观察并按自己的资格推进。

首个候选完成 Task claim；后续候选不再次领取或重开 Task。Task 仍为 in_progress，只有全部前置已交付、契约允许 multiple、没有交付/丢弃/任务级变更冲突、服务已恢复就绪且人工派发未暂停时，才可启动追加候选。所有本次领取下的候选共享首次 claim 固定的仓库、目标分支、契约摘要和 base；逐个核验目标端可解析该 base，分支及 worktree 必须不同。安全重开后下一次领取可以取得新 base，但不得修改旧候选的 base。

`start_candidate` 接收 task_id、request_id、expected_task_version、expected_graph_revision 与选定执行配置。同 ID/内容返回原候选/启动回执，异内容拒绝；不同 ID 可创建不同候选。事务重新核任务、图、前置 Delivery/base 证据并保存候选身份及外部意图，网络启动在事务外进行。并发版本竞争的失败方返回 stale_version，不自动改版本重发。启动回执丢失时按原 Operation 对账，不能再次派发未知动作。

## 独立流程与读模型

清除按 task_id 选“最后一个运行”作为唯一事实的 Service、投影、worker 查找、停止/重试、恢复和输出路径。答案继续绑定 Decision/Attempt/Execution；所有执行级变更命令必须提供精确 workflow_run_id 及原有版本/身份。旧任务级命令若无法唯一定位候选，返回 ambiguous_execution，不能猜最近一项。

运行投影逐个按 workflow_run_id 重建，task_id 只用于取得候选集合或任务级事实；查找时核验 WorkflowRun 属于指定 Task、Execution 属于该 WorkflowRun、Attempt 属于该 Execution。既有 worker 映射继续按 WorkflowRun 管理，不复制到另一套候选调度器。任务级占用摘要和 UI 选择不能代替执行身份，也不能据此遗漏兄弟候选或清除其 unknown 占用；历史候选与当前 claim 的候选分开解释，旧结果不得改写当前候选。

QueryService 提供 Task 下稳定分页的候选列表和精确候选详情，返回 task_id、workflow_run_id、execution_id、attempt_id、对象版本、阶段、后端/目标、base、workspace、能力、Decision、PR、资源状态和阻塞原因。列表包含 items、next_cursor、has_more、database_revision；游标失效不能拼接新旧快照。声明未交付能力时返回 unsupported，不伪造空成功。#27 在同一 DTO 中填充 delivery_role（unselected/winner/loser/conflict）及结算结果；#26 阶段以 unselected/unsupported 表示，不另建并行查询协议。事件持久保存所属 Task/WorkflowRun/Execution/Attempt 后供消费者补读。

为保证本任务可独立合入，在 #27 仲裁能力交付前，multiple 任务的候选可实现、验证和审查，但所有发布 PR、TaskDelivery 和后继释放入口均在服务端拒绝并给出 `candidate_delivery_unavailable`；单候选原流程保持可用。该门槛由实现能力决定，不提供可由用户绕过的配置开关。

## 恢复与状态保护

实现共同约束规定的受控格式转换，保留真实原 Task/DAG 和未结算外部身份，旧运行不重派。转换须升级格式标记，使旧二进制拒绝新格式的业务写入；原契约缺策略字段的解释只限该明确转换输入，不开放任意旧库读取。恢复逐一读取所有候选，不以最新一项覆盖其他候选；缺少精确身份的原执行保留 unknown。修改数据库唯一索引时，仍由事务保证每 WorkflowRun 只有一个活动 Execution 和每 Execution 只有一个活动步骤尝试。

尚未领取、没有任何候选/发布意图的 open multiple Task 与 single 一样支持 add-predecessors 和 discard，不因 execution_policy 拒绝。已领取 Task 的 reopen、补前置并重置及 discard 必须核对全部本次候选：精确 worker 已停止/释放、没有未结算启动/回答/发布 Operation、没有待处理 Decision、没有 PR 或无法排除 PR 的发布行为。平台未知或活动资源返回 cleanup_pending 并保留原状态；不存在“人工确认即可清掉 unknown”的入口。候选均失败且已满足这些条件时可以重开或丢弃，不强迫继续添加候选。

事务重新核验 Task 版本和全部候选集合/状态版本，与新候选准入互斥；重开或补边时统一将旧候选标记 abandoned，保留所有历史和工作成果，Task 变为 open、清除当前 claim 并提升版本，补边在同一事务提交。旧结果不能推进新领取；后续首次 claim 使用新前置及新 base。丢弃沿用 discarded 语义，不释放后继。任何已知 merged PR/Delivery 均拒绝上述操作，存在 PR 的资格由 #27 扩展。Task 已有交付/丢弃事实时拒绝追加候选。#27 接入后，启动准入与交付事务仍在同一 Task 版本上竞争。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| CM1 | 同 Task 创建三个独立候选，不存在业务数量上限；不同配置/工作区/请求/问题互不串线，single 策略拒绝第二候选 |
| CM2 | 首次 claim 只发生一次；相同请求重放只得原候选，异内容拒绝，版本竞争和启动回执丢失不重复 worker |
| CM3 | 各候选逐步运行；按 Task 猜执行的命令拒绝，回答不能作用于兄弟候选；候选顺序变化、历史候选迟到结果、分页与重启恢复不遗漏或替换候选，unknown 占用保留 |
| CM4 | 新候选 base/契约与首次 claim 一致；依赖或任务版本变化、目标端缺 base、工作区重复均在副作用前阻断 |
| CM5 | multiple 发布/交付在 #27 前全入口受控拒绝；open 零候选可补边/丢弃；全部已结算失败候选可原子重置；任一活动/未知候选、集合竞争或未决问题均阻断，旧结果不推进新领取 |
| CM6 | 固定 schema 5 样本转换前后原 Task/DAG、契约字节/摘要、未决问题、unknown Operation 和外部绑定保持；含已领取/等待 PR 的原任务按旧摘要模式继续校验和恢复；新契约策略改动导致摘要失败，未知模式拒绝；中断不重建库或派发替代执行 |

CM1–CM6 使用隔离项目与后端替身验证中心行为，真实 local/Orca 多候选证明归 #29。验证记录遵守共同约束。
