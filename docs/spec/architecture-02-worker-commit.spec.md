# Spec：架构收口 PR 2 — worker 生命周期显式原子提交

状态：待实施；未分配 Task ID。日期：2026-09-30。审计基线：`main@4e786d7`。硬前置：[PR 1：worker 协议与终态边界](architecture-01-worker-protocol.spec.md) 已合入；本 Spec 由一个实现 PR 交付，后继为 [PR 3](architecture-03-runtime-boundary.spec.md)。

## 1. 目标与依据

worker 每次业务转换明确声明读取前提、修改记录、操作和事件，并在一个 SQLite 事务内提交。外部动作只能使用已成功持久化的意图；中心不能通过发布整份 Runtime 顺带保存这些业务变更。

遵守 [worker-flow](worker-flow.spec.md)、[原生 Orca](worker-flow-orca-native.spec.md)及[原生任务共同约束](native-task-constraints.md)。复用 `transactSQLite`、现有领域记录、Operation 幂等规则和 `ControlRevision`。不新增通用仓储、事件溯源、跨系统事务或旧数据兼容设施。

## 2. 范围与分工

当前 `Store.Publish` 经 `saveRuntime` 扫描 WorkflowRun、Execution、Attempt、Decision 指纹，写回差异并保存 Operation、领域事件与 checkpoint。`commitWorkerFeedbackPhase` 已显式提交一个反馈阶段转换，但其余正常流程仍依赖隐式写回。

本 PR 建立提交入口，并将三种 worker 模式的正常生命周期完整接入：

| 流程 | 本 PR 必须显式提交的内容 |
| --- | --- |
| 准备与绑定 | workspace、process/session、coordinator 等固定绑定及其操作意图/回执；领取仍由已有原生命令事务负责 |
| 开始轮次 | pending command/turn、Attempt 状态、发送意图、平台接受/开始回执及未知结果 |
| 等待与决策 | 等待阶段、Decision 创建/替换/状态、问题队列与 Execution 诊断 |
| 答案与反馈 | 原问题回复、轮次后的新反馈、waiting_pr_merge 到反馈阶段转换、投递回执及失败/未知记录 |
| 轮次结束与 PR 观察 | PR 1 的终态事实、轮次状态、PR 候选/核验结果、待合并反馈与相关事件 |

首次启动及正常运行中收到的失败/未知结果属于本 PR。服务重启后的恢复调度、后台对账、最终资源清理和其余服务/控制写入由 PR 3 迁移；它们复用本 PR 已有转换时，必须复用同一提交入口。Task 完成仍使用既有 TaskDelivery 原子事务，不通过通用记录覆盖替代其资格核验。

## 3. 提交契约

增加内部有类型的提交请求与结果，具体命名可由实现确定。请求限定于一个项目及明确关联的 WorkflowRun/Execution，包含：

- 本次转换需要的固定业务身份、预期 `ControlRevision`，以及参与判断或更新的记录前值/指纹。新增记录明确要求原身份不存在。
- 显式列出的 WorkflowRun、Execution、StepAttempt、Decision、Operation 变更；记录未列入就不能被该提交写回。
- 本次转换产生的领域事件和必要的业务消费标记，以及提交后对应的 ServiceCheckpoint 数据。

不能传入整份 Runtime 再扫描出“本次修改”，也不能用调用前后 diff 自动补齐写集合。单条记录指纹可用于并发校验，其用途是证明前提仍成立，不是发现写入对象。

事务先检查项目与记录关联、读写前提、幂等身份及合法状态转换，再写入声明的记录、操作、领域事件和 checkpoint。校验与写入必须在同一事务中；提交不能在业务写完后另起事务发布成功 checkpoint。相关 Task 已完成/丢弃、当前 Attempt 已变化等事实也必须在事务内检查，不能只信任 actor 缓存。

沿用现有数据库/控制版本推进规则。新 Operation 的幂等键与请求摘要冲突应拒绝；重放不得生成另一 operation 身份、重复事件或把 confirmed 回退为 accepted。不要因只更新诊断而新增控制状态变化。

## 4. 提交结果与外部副作用

1. Service 在局部副本上计算转换；成功后用提交返回的权威记录与版本更新 actor 和读视图。失败不把候选状态标为已持久化，不清空待保存事件，不发送后续外部动作。
2. 若持久化冲突，保留已保存的答案、原意图和现场，按既有错误/恢复策略停止该动作或重读；不得用旧 actor 状态覆盖并发命令，更不能无条件重试整次外部调用。数据库错误继续沿用既有持久化失败阻断。
3. 外部调用前，在一个提交内持久化必要业务状态及操作意图；事务结束后才调用平台。平台返回的 accepted、confirmed、rejected、outcome_unknown 再通过显式提交记录，不把网络/进程调用放入 SQLite 事务。
4. 意图落盘后进程退出、请求已执行但回执丢失时，已有恢复路径必须仍能找到原 Operation 和关联身份。缺少回执不能等同于未发送。
5. 消息的业务状态和领域事件落盘成功后才确认消费。原始日志文件不承诺与 SQLite 跨资源原子提交；独立的会话输出事务可继续存在，但展示游标不能被当成业务已处理证明。若某来源游标决定业务是否重放，其消费标记必须与本次业务转换一同提交，保证重启不会漏处理。
6. `commitWorkerFeedbackPhase` 的固定身份、版本和保存反馈后才发送的语义纳入同一边界，避免阶段提交与下一次 Runtime 发布争用同一记录。移除已被替代的专用实现，或仅保留委托同一事务逻辑的薄入口。

## 5. 实施落点与过渡边界

先交付提交结构、事务校验与原子性测试，再迁移准备/轮次意图、事件/决策、反馈和终态/PR 观察。`operation.go` 的 helper 要能与所属业务转换组合提交，不能内部调用全局 `s.save()` 意外提交调用方尚未完成的状态。

主要涉及 `sqlite_store_runtime.go`、`store.go`、`state_revision.go`、`operation.go`、`worker_feedback_phase.go` 和三种 worker 的业务接线。PR 1 提供的纯事实契约不增加数据库依赖。

本 PR 结束时，未迁移的恢复/清理等调用点可继续使用当前隐式入口，必须在 PR 描述中逐项列明其归属并交给 PR 3。已迁移路径只走显式提交，提交失败不能回退到 `saveRuntime`，同一转换不能显式提交后再双写。旧入口仅服务尚未改造的当前代码，不读取或兼容历史版本数据；PR 3 必须删除它。

显式提交成功后，仅同步已提交记录与尚存发布入口使用的对应基线，避免后续发布将其重新识别为待写变更；不能顺带把其他未提交内存改动标为已持久化。提交失败不推进任何基线。此过渡接线随 PR 3 的隐式入口一起删除。

## 6. 最小验收

| 编号 | 必须验证的结果 |
| --- | --- |
| WC1 | 修改一份未列入写集合的内存记录不会顺带持久化；声明的业务记录、Operation、领域事件及 checkpoint 同时提交 |
| WC2 | 读写前提冲突，或在事件/checkpoint 写入阶段注入失败，整笔事务回滚，内存视图不报告成功；意图提交失败时平台调用次数为零，回执提交失败时不继续后续副作用并保留原意图供对账 |
| WC3 | 本次请求使用的意图在平台收到调用前已可从数据库读到；回执丢失/重启仍关联原请求，重复输入不产生第二次发送 |
| WC4 | 并发保存的答案不被旧 actor 覆盖；待合并反馈及连续问题保留原 Execution/Attempt 和问题身份 |
| WC5 | 三种模式的准备、轮次开始、问题/反馈与终态均有显式提交接线；成功、明确失败和结果未知保持原语义 |
| WC6 | PR 关闭未合并不能交付；真实 merged 证据仍经既有唯一 TaskDelivery 事务关闭任务并释放后继 |

复用 `TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor`、`TestPublicationPreservesExternallyAcceptedDecision`、`TestPublicationRejectsConflictingDecisionAtomically`、`TestAppServerAnswerRecoveryPersistsBeforeNextQuestion`、现有 local/Orca fixture 和 worker-flow 交付测试；旧 publication 测试迁到显式提交入口，保留其并发与回滚断言。新增测试集中验证写集合、事务末端失败和外部调用前持久化这几个边界，不重复铺开平台故障矩阵。

只运行受影响的存储、worker 与交付针对性用例；不默认要求全量套件或新的真机能力认证。PR 记录实际命令、覆盖的 WC 编号、失败/跳过及 PR 3 尚待迁移的入口。

## 7. 完成条件

正常 worker 生命周期不再依赖 Runtime 指纹扫描保存业务变更。每个外部动作都能定位到其已提交意图和回执写入点，每个业务转换都能说明读前提与写集合。[PR 3](architecture-03-runtime-boundary.spec.md) 据此完成剩余调用点迁移和读写模型分离。
