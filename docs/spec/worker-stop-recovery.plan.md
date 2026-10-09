# Plan：Worker 停机对账与恢复一致性修复

任务 ID：87；任务名称：Worker 停机对账与恢复一致性修复。

状态：已获用户授权由本聊天直接执行；启动固定文档版本，不经 roadmap-runner。依据：[Spec](worker-stop-recovery.spec.md)；结果登记：[专属验收](../acceptance/worker-stop-recovery.md)。启动另固定实际文档 commit/路径及最新主干；旧诊断源码不代表可跨构建接管。

## 实施安排

1. 在现有 [worker 生命周期端口](../../internal/execution/backend/lifecycle.go) 声明 `ServiceStopBehavior` 两值；具体 worker 实现声明值，冻结于已有 [FixedExecutionProfile](../../internal/execution/model/execution.go)，摘要/版本核验同步。没有值或能力不匹配在副作用前拒绝；不以当前 Profile 或 backend 名判断。升级受影响持久/冻结版本，不兼容旧格式。worker 接线与初始化/恢复共用原冻结能力，不依赖当前存活的观察器对象。
2. 在 [worker](../../internal/execution/worker_types.go) 与已有 lifecycle 接线收口服务停机：Service 持久门槛后调用 worker 入口；local 实现释放精确 owned Job/进程组，删除停服 interrupt RPC；Orca 实现只脱离原观察。Service 不按 protocol 分支或直接操作实现进程树。准备阶段 owner、晚到创建回执及取消后资源回收由同一 worker owner 处理；有界清理，错误不跳过其他 worker。已通过持久屏障的原 Delivery ack 使用独立有界上下文完成，停机 actor 仍消费其原不可变回执；业务输入不享有此收尾例外。候选放弃/丢弃 stop 接线不改变业务语义。
3. 将 [app-server 专属恢复](../../internal/execution/worker_lifecycle_recovery.go) 的原生 probe、身份/进程树与 journal 解码收进具体 worker 生命周期；[通用路由](../../internal/execution/worker_lifecycle.go) 只调用统一 worker 恢复入口。复用已有 Reconcile/Observe/VerifyManualRecovery 能力与规范事实；Orca 的原 terminal/Dispatch、待问题和离线 worker_done 由其实现核实并接回。原生 worker-start 的未知结果只能 request-show 只读对账，缺原回执不创建/重试输入；精确原输入已accepted与实际turn已started分别处理。支持格式下现存停止意图由 worker 解析/返回标准结算，上层仅验证通用身份和提交。不另建接口框架、后台业务写者或恢复权威。
4. Service 统一接收规范结果并使用已有 [ClassifyRecovery](../../internal/execution/model/recovery.go) 和 [未知操作门槛](../../internal/execution/worker_recovery.go)，保持 [答案](../../internal/execution/worker_feedback.go) 受理/投递资格、版本/first-wins/换代事务。原结果按既有业务/ack 规则提交，原生已完成不能被标识或进程退出覆盖。相同事实不重复 commit/note；迟到结果校验代次。停止或继续的标识都是预期，不能代替运行事实。Query 优先复用已有原因/动作，不做 HTTP/WinUI。
5. 以同一通用 worker fake 的两种标识覆盖上层规则，具体 local/Orca adapter 测试证明资源/身份/结果事实；产品集成同时测两路径。真实 local 用例扩展正常停服分支，Orca 复用已有 recovery live 场景补活动停服及离线完成窗口。原生验收驱动在产品原准入期限内提交同一正式请求，受理后有界等待真正最终 applied 回执；不把 applying 当业务开始/交付，不因等待期限重发。验收写入专属记录、纳入后同步 execution-map；独立复审及真实 PR 合并核验后才记实现完成。#84 共用恢复契约有序集成，不静默修改其已固定规格。

## 测试计划

[WSR-A1–A8](../acceptance/worker-stop-recovery.md) 全部映射如下。测试及现有测试的新分支按以下最小范围实现；验收前核对实际子场景匹配。每次 runner 使用新临时目录，原始日志不入仓库，零匹配/skip/缺证据不算通过。

| 验收项 | 级别 / 组或场景 | 环境 / 命令 | 时点 / 通过条件 | 证据与登记 |
| --- | --- | --- | --- | --- |
| WSR-A1–A5 | 定向 owner / `TestWorkerServiceStopRecovery`、`TestWorkerServiceStopBoundary`，必要原未知发布/兄弟回归 | 隔离 SQLite、worker fake 两标识、受控 managed 父子与 Orca 协议；`go run ./cmd/test-feedback -- ./internal/execution -run '^Test(WorkerServiceStopRecovery|WorkerServiceStopBoundary|WorkerShutdownDrainsCommittedDeliveryAck|WorkerPreparationStopDoesNotJournalUnsentBusinessTurn|WorkerLateReplyReleasesOnlyItsOriginalUncertainty|DeadWorkerRecoveryPreservesDecisionAndFreshAnswer|CandidateResumeErrorDoesNotStarveSibling)$' -count=1` | 契约/接线后；冻结/缺值/版本拒绝、只消费统一事实、两种停服动作、身份/未知/答案/重复/隔离完整覆盖 | 子场景和边界枚举、Operation/资源/发送计数、runner summary；登记 A1–A5 |
| WSR-A2 / A3 / A5（实现事实） | 定向 worker adapter；`TestServiceStopWorkerFacts`（各 adapter 包） | local 的精确 managed 资源与 Orca 受控原 runtime/terminal/Dispatch/结果/问题；`go run ./cmd/test-feedback -- ./internal/execution/adapters/codex ./internal/execution/adapters/orca -run '^TestServiceStopWorkerFacts$' -count=1` | 具体实现完成后；local 全树退出、不发 interrupt；Orca 不关 terminal，不丢离线完成/待答；身份不明保持未知 | 原绑定、原生样本及真实受控资源退出/调用计数；登记 A2/A3/A5 |
| WSR-A6 | 受控产品集成 / `TestWorkerServiceStopRecoveryControlled` | existing integration 隔离服务/正式 Command/Query、local/Orca helper；`go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerServiceStopRecoveryControlled$' -count=1` | 两 worker 分支和夹具补齐、最终候选；local 停止→重开不发送→正式继续新 Attempt 同会话；Orca 原运行/待答/离线完成分别重接，原 Attempt/Dispatch，不重复启动/发送/结算；二次重启和清理通过 | 同快照 Query、Command/Decision、原/新身份、start/send/stop/settlement 计数；登记 A6 |
| WSR-A7 | 原生 opt-in / 现有 `TestLocalCodexManualRecoveryLive` 扩展正常停服分支 | Windows、目标 Codex/认证、隔离 home/Git/SQLite、现行权限；独立进程设置 `STEERLINE_WORKER_RECOVERY_LIVE=1`，`go test ./tests/integration -run '^TestLocalCodexManualRecoveryLive$' -count=1 -timeout=15m` | 真实分支/前置就绪、最终候选；旧受管树停止，重开零新 turn；正式 continue 同 thread/worktree、新 Attempt/进程/turn 完成；保留 host-exit 回归 | exe/CLI/构建、原生身份/事实、Command/usage/清理；登记 A7 |
| WSR-A8 | L3 live / 现有 `recovery.orca-codex` 扩展活动/离线完成分支 | Windows、本机 Orca/Codex、独立 home/项目、loopback provider；`go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex'` | 最小离线窗口驱动就绪、最终候选；活动停服不关原 worker，重开仍接原 Attempt/terminal/Dispatch；再停服让原轮次完成，重开接回可信结果、按既有规则结算一次，不重发业务输入；清理通过 | 原 runtime/incarnation/terminal/Dispatch、原轮次输出/worker_done、Query、发送/结算计数、usage；登记 A8 |
| 必要回归 | L1 / 既有依赖边界 | `go run ./cmd/test-feedback -level=L1`；按实际移动接线定向补现有 owner/boundary 守卫 | 最终候选；单写/冻结/事务及消费者保持，应用恢复无原生分支 | runner summary，不额外计产品验收 |
| 规划静态 | doc-links / identities / whitespace | `python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 文档修改后；导航/身份/格式正确 | 交付说明，不填产品已验 |

能力补齐：现有 controlled helper 增加继续型 worker 的离线运行/完成/待答、原回执与计数；fake 与真实实例都走 worker 新入口。现有 Orca live 场景增加隔离任务的最小可控等待窗口：先重开核实仍活动且未重派，再次停服释放测试窗口让原轮次完成并接回结果。窗口只影响本次合成任务，不重新给 worker 发送业务提示；完成来源仍须真实 worker 输出，不能注入假 worker_done。若现有 driver 无法控制窗口，在 native/driver 侧补最小能力，不在业务 Service 加测试专用分支。

Orca 离线原轮次继续执行允许同一轮 usage 增长；证明不重派用原会话/Dispatch/turn 与业务发送次数，不能把“usage 不增加”用于活动场景。恢复前后无新增业务输入、停服期间可信输出及结算/ack 分别留证。A7/A8 用当前原生权限，所有资源精确归属/清理，不操作安装 101 或现有 Orca 终端。

不跑 Full、完整 L2、新增 suite 或全崩溃矩阵。双 worker 语义为本次核心要求，因此 local/Orca 各一条最小真实链；受控分支覆盖待问题、迟到和未知故障。无新增人工项，原一期组合/长期签收保持独立。
