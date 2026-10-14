# Plan：worker 运行时资源所有权收敛

任务 ID：96；任务名称：worker 运行时资源所有权收敛。

依据 [Spec](worker-runtime-ownership.spec.md)，实时结果归[专属验收](../acceptance/worker-runtime-ownership.md)。2026-10-14 草稿；调查基线/RoadmapGuide/SpecGuide版本 `5a24abec3c10d184a6e57962e154854bc6011a9f`，启动另固定实际文档与最新代码。

## 实施与删除安排

1. 列出Service的workers/ownedProcesses/workerServiceOwners/lifecycleEffects、forwarders以及恢复中的临时observer所有权。按物理资源区分句柄/观察/在途调用与领域/回执处理；记录owned/attached、冻结停服策略、原generation、退出/ack的现有屏障。不重写adapter证据和业务恢复资格。
2. 新建 `internal/execution/workerruntime`，只消费backend值/标准库及本消费者的Execute/Reconcile、Observe、Stop/关闭等小端口。建立当前所需的原资源owner和在途调用状态；接收已持久意图，保留原后台调用机制，不增加消息总线或通用scheduler。返回固定request身份的结果/资源引用，未知只对账。
3. 迁移workerFromLifecycle事件桥接、观察关闭、原句柄与在途调用的资源map/取消/完成交接。actor仍提供已有Signal入口并最终接纳/拒绝结果；用具体操作kind/身份分派原业务结果处理，不把接收可写RunView的apply闭包传入runtime。删除旧资源map/旧Worker桥接及重复运行时调用，actor只保留业务等待/身份引用。
4. 修改worker_lifecycle/recovery/shutdown/cleanup的资源接线；保留原资格、领域状态、persisted/processed确认与提交屏障。事件在成功入原actor队列之前由runtime持有失败通知权，成功入队原子地转给actor；不能依据之后ctx是否取消推翻已发生的交接。未交接取消/失败由runtime向未完成确认报error；入队后的过期结果、提交失败和停服排空由actor处理。Persisted/Processed各自保留一次通知状态，已Persisted成功后处理失败只通知Processed失败，其关闭也由唯一owner负责；runtime从不发nil成功。复用原队列和排空机制，不建设新ack协议/总线。明确未交付observer、accepted但未保存的资源出口由谁释放，取消/等待转发/排空不得形成actor等待自身消费的环。动作kind分派不改变原动作选择/重试/恢复策略。
5. 新增纯runtime `TestWorkerRuntimeOwnership`、boundary的 `TestWorkerRuntimeBoundary` 与actor的 `TestWorkerRuntimeCommitBarrier`（当前不存在）。以可控小端口/通道覆盖并发原资源、unknown不重发、旧Attempt/generation、owned/attached、关闭/取消/一次释放、挂起事件及无残留。用同步屏障确定性安排交接前取消、入队成功后同刻取消、排队后迟到/错代次、提交失败及停服排空；逐道断言成功不提前、失败会通知、每道只一次且Processed只关闭一次、无阻塞goroutine。提交屏障用现有SQLitefixture注入失败，还验证已Persisted成功但处理失败不重发Persisted，已提交ack停服仍收敛，一个慢observer不阻塞另一原运行。边界独立文件扫描all-tag源码与Windows/Linux闭包，禁止父execution/task/product/storage/app/adapter/development/delivery；不放宽旧白名单。
6. 新增本项actor/SQLite fixture的 `TestWorkerRuntimeDeliveredCleanup`（当前不存在），沿原正式交付入口完成真实受控merge核验，注入资源清理失败及后续原操作对账；断言Task仍closed、TaskDelivery仅一个且证据不变、图/后继资格保持、未再次派输入/交付、清理错误和原资源身份被保留。不依赖#95新包或测试helper，可在现有CompleteNativeTask入口独立交付。运行原阶段推进、停服/迟到/提交回滚与四个注册controlled恢复场景，并按条件组合表复验已合入#94/#95；driver/adapter/native/protocol代码冻结，必要测试仅适配资源引用类型，不能削弱身份或清理断言。新runtime通过普通Go定向runner执行，不新增业务场景/环境。
7. 自查每个原物理资源只有一个关闭owner、runtime没有业务能力或指针、旧状态已移除。按测试表登记源码/构建/结果/绝对证据到专属验收，更新架构/并行指南。正式纳入后同步execution-map真实PR/验收；草稿不派发。

## 开工与共享接线

没有新增硬前置；#94→#96为默认集成建议，不是执行依赖。本项不改staged_flow/development规则或#95最终交付用例；service_commit/domain/committed及资格/模型冻结。共享Service结构中的运行时段由本项唯一修改；#94只接纯规则，#95只接最终交付。三个任务公共签名/业务字段不变时可独立验证；若需兄弟未交付新字段/接口，启动前重定真实依赖，不能按建议假定接口已经存在。建议#94/#95后合入本项，最后合入者补最小组合回归。

最终候选记录实际合入基线commit及#94/#95成果是否已在其中；自身阶段/清理测试始终运行。#94已合入时运行其未知发布合并反例，#95已合入时运行其真实事务测试，条件未满足时明确记录“基线未包含该成果，条件未触发”，不运行不存在的名称、不登记为通过、不等待它。条件满足却缺测试/零匹配则失败。若之后兄弟后合入，它按自己的Plan复验相关交接；固定L1不替代这些定向组合。

## 测试计划

新包/测试是待实施入口，先由步骤2/5补齐并枚举，零匹配/skip失败。每行runner产生新的系统临时 `steerline-test-feedback/<运行时间>/`，summary.json和原始Go日志/stderr按行标识登记到本项验收“执行记录”；E2E场景记录原资源及清理事实。

| 验收项 | 级别/组/场景、环境 | 命令 | 时点/通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [WR96-A1](../acceptance/worker-runtime-ownership.md#自动验收计划) | 定向新runtime/新增TestWorkerRuntimeBoundary；小端口、Windows/Linux源码闭包 | `go run ./cmd/test-feedback -- ./internal/execution/workerruntime -run '^TestWorkerRuntimeOwnership$' -count=1`；`go run ./cmd/test-feedback -- ./internal/testkit/boundary -run '^TestWorkerRuntimeBoundary$' -count=1` | 提取后、PR前；单一资源owner、无业务能力/旧闭包/实现依赖；Linux不宣称运行验证 | 两个runner目录；WR96-A1 / owner、boundary |
| [WR96-A2](../acceptance/worker-runtime-ownership.md#自动验收计划) | 定向资源/原停服与迟到；小端口及隔离受控进程fixture，消费WR96-A1资源反例 | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerServiceStopRecovery|TestWorkerServiceStopBoundary|TestWorkerLateReplyReleasesOnlyItsOriginalUncertainty|TestWorkerPreparationStopDoesNotJournalUnsentBusinessTurn|TestLifecycleEffectForwardsWorkspaceCreationCheckpoint)$' -count=1` | 资源接线后、PR前；原策略/身份、未知不重发、一次关闭/释放与退出收敛 | runner目录；WR96-A2 / resources |
| [WR96-A3](../acceptance/worker-runtime-ownership.md#自动验收计划) | 定向actor/事务；新增屏障/交付后清理及原失败fixture，同步屏障+实际SQLite | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerRuntimeCommitBarrier|TestWorkerRuntimeDeliveredCleanup|TestWorkerShutdownDrainsCommittedDeliveryAck|TestWorkerCommitRollsBackRecordsReceiptsEventsAndCheckpoint|TestSimplifiedLifecycleActorDoesNotWaitForEffect)$' -count=1` | 屏障/清理接线后、PR前及最终候选；成功不提前、未交接失败由runtime/入队后由actor恰好一次通知，已提交ack排空；清理失败保留closed/唯一Delivery/后继资格，慢调用不阻塞actor | runner目录；WR96-A3 / commit、ack、delivered-cleanup，登记各交错及清理反例 |
| [WR96-A4](../acceptance/worker-runtime-ownership.md#自动验收计划) | L3 controlled / recovery.observation-reconnect、terminal-local-reconnect、terminal-environment-unknown-send、answer-replay；已注册隔离driver | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.observation-reconnect,recovery.terminal-local-reconnect,recovery.terminal-environment-unknown-send,recovery.answer-replay'` | PR前及有共享actor构造变化的最终候选；四个实际scenario通过/cleaned，原会话/终端/输入保持，重交不增加副作用 | E2E runner目录；WR96-A4 / scenarios |
| [WR96-A4](../acceptance/worker-runtime-ownership.md#自动验收计划) / 原阶段组合，始终运行 | 定向controlled app，既有Codex/Orca local/environment三个入口；独立fixture | `go run ./cmd/test-feedback -- ./tests/integration -run '^(TestWorkerStageLocalRoundTrip|TestWorkerStageOrcaTerminalRoundTrip|TestWorkerStageOrcaEnvironmentRoundTrip)$' -count=1 -timeout=15m` | PR前最终候选；三阶段经原actor与新runtime保持会话/Attempt、正文/次数/步骤，无skip | runner目录；WR96-A4 / stage-assembly |
| [WR96-A4](../acceptance/worker-runtime-ownership.md#自动验收计划) / #94已合入时 | 定向actor/SQLite/provider，#94交付的未知发布合并反例 | `go run ./cmd/test-feedback -- ./internal/execution -run '^TestDevelopmentUnknownPublicationMergedDelivery$' -count=1` | 最终候选基线含#94时必跑；未知原PR merged经规则/actor/runtime仍唯一交付，错PR拒绝 | runner目录及实际基线commit/触发条件；WR96-A4 / development-combination |
| [WR96-A3](../acceptance/worker-runtime-ownership.md#自动验收计划) / #95已合入时 | 定向真实SQLite提交与本项清理反例；#95事务测试+#96 fixture | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestDeliveryAtomicCommit|TestWorkerRuntimeDeliveredCleanup)$' -count=1` | 最终候选基线含#95时必跑；唯一交付/失败回滚、已交付后清理失败不重开Task，已有相同候选执行证据可复用 | runner目录及实际基线commit/触发条件；WR96-A3 / delivery-combination |
| 必要回归 | 固定L1 | `go run ./cmd/test-feedback -level=L1` | PR前一次；既有公共边界通过 | L1目录；回归记录 |

不跑Full、L2/live/native：本轮不改具体adapter、平台命令/证据、权限或原生能力，资源与提交行为由小端口、隔离进程、真实actor/SQLite及双目标controlled覆盖。若不得不改变原生动作/停服策略，应先重新定范围并补独立live验证，不能拿此说明豁免。无新增人工窗口/业务验收任务。
