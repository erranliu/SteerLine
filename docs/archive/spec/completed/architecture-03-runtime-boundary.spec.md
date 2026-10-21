# Spec：架构收口 PR 3 — 移除隐式写回与读写模型收口

状态：实现候选，待本 PR 评审；未分配 Task ID。日期：2026-09-30。实施基线：`main@d809a45`，承接已合入的 #147、#152 与 #156；本 PR 补齐最终读视图、协议事实和绑定/停止事务边界。

## 1. 目标与依据

所有业务表写入都由显式命令或转换事务负责；Runtime/Run 只承担读视图，checkpoint 发布只写服务快照。完成剩余恢复、清理和服务控制调用点迁移后，删除 Runtime 整体差异写回，让改变内存视图不再可能触发业务写入。

同时按实际调用方收窄执行后端接口，结束中心业务流程与平台协议的交叉依赖。遵守 [worker-flow](worker-flow.spec.md)、[原生 Orca](../retired/worker-flow-orca-native.spec.md)和[执行架构](../../../spec/execution-architecture.spec.md)。本 PR 不新增旧数据迁移、双写、兼容开关或隐式提交回退。

## 2. 剩余写入点迁移

实施时先接收 PR 2 的剩余入口清单，并追踪业务字段赋值、`s.save()`、`Store.Publish`、`SaveRuntime`、Operation 与 PendingEvents 的生产者；不能只通过搜索 `save` 来判断所有写入都已迁移。

| 类别 | 必须完成的迁移 |
| --- | --- |
| 重启恢复与后台对账 | app-server 启动/回答日志恢复、原生 Orca request/Dispatch/Delivery 对账、受监督执行恢复、原身份接回及未知结果更新 |
| 停止与资源清理 | 停止/关闭意图、原资源身份核验、清理回执与未知记录；TaskDelivery 后的清理仍独立记录 |
| 服务与控制接线 | `service.go`、`control_service.go`、`native_service.go`、`command_processor.go` 等路径中的业务修改；控制操作转 Operation 的写入显式化 |
| 观察与诊断 | Execution 诊断、输出来源/游标及会话消费位置；属于业务记录的字段继续经显式提交，服务 heartbeat 单独发布 |
| 测试与初始化 | 依靠 `SaveRuntime` 顺带写入业务记录的 fixture 改为明确建数，不能保留测试专用生产写回捷径 |

恢复与正常执行共用 PR 2 的提交入口和前提校验，不建立第二套恢复写者。命令、领取、回答和唯一 TaskDelivery 已有的专用事务保持权威；不强制把所有业务表放入一个任意 CRUD 接口。

## 3. Runtime 与 checkpoint 边界

Service 私有的领域快照/工作记录与对外 Runtime 投影明确分开。Runtime 可继续包含查询所需业务字段，但不能作为提交请求或可写业务记录容器。查询返回的 slice、map 和指针不能让调用方反向修改 actor 待提交数据；使用有需要的复制和转换即可，不建设通用实体跟踪框架。

| 入口 | 收口后的契约 |
| --- | --- |
| 命令/转换提交 | 显式读前提与写集合；相关业务记录、操作、领域事件、业务消费标记与 checkpoint 在事务内一致提交 |
| `s.save()` / `Store.Publish` 或其替代命名 | 只接受服务 checkpoint 所需输入，只更新服务快照/发布时间，不扫描或更新业务记录、Operation 或领域事件 |
| heartbeat | 更新服务存活信息，不推进业务控制版本、不确认外部操作 |
| Runtime/Run 构建与查询 | 从已持久化的领域记录和 ServiceCheckpoint 构建；数据库查询使用同一读取快照，内存投影使用已提交记录 |

checkpoint 不能把未提交的业务状态包装成成功视图，也不能在服务刷新时覆盖刚被命令事务更新的答案或控制信息。外部命令提交后，Service 在下一次相关判断前读取并校验最新权威状态，不能以较新的 checkpoint 版本冒充较新的业务状态。

保留当前格式重启恢复：从 SQLite 恢复领域事实、原 Operation 和服务快照，再核对原 worker。未提交的内存改动不参与重启恢复；不得从旧的 Run 投影反推缺失业务记录。

## 4. 删除隐式入口

删除 `saveRuntime` 中对 WorkflowRun、Execution、Attempt、Decision 的全量差异扫描，以及通过 Runtime 自动收集/写入 Operation、PendingEvents 的逻辑。生产与测试调用均迁完后，删除原 `SaveRuntime` 写回 API 或替换为签名明确的 checkpoint API。

移除只为隐式发布服务的 `Store.baseline`、`domainFingerprints`、publication baseline 校验和关联刷新代码。显式提交所需的单条记录指纹、`ControlRevision`、交付冻结证据、Operation 幂等与合法状态校验必须保留，不能把删除扫描理解为删除并发保护。

逐个审计 PR 2 遗留入口，确认其写入走显式事务或确实只发布 checkpoint。最终代码中不存在“修改任意 Runtime 记录后调用 save 即持久化”的路径，也不存在失败后切回旧入口的分支。

## 5. 后端接口收口

依据生产消费者整理 `ExecutionBackend`、worker mode 路由与协议适配入口：只保留当前调用方需要的职责；确有不同消费者时拆成小接口，不为了形式统一强迫每个后端实现永远 unsupported 的方法。

准备、启动、反馈、观察/终态和恢复证据读取均以固定 Execution 上下文进入相应适配层；中心负责业务调度、意图和提交，平台层执行固定动作并返回事实。显式能力拒绝继续存在，不能把 unsupported 改为成功空操作，也不能在能力不足时偷偷切换后端。

中心业务状态机不解析原生 JSON、日志文件、method 名或终端文本。平台层不持有业务写库权，`workerflow` 纯规则不依赖数据库或平台。引用关系检查以实际 import 和调用链为准，不以文件改名、代码行数或接口数量作为验收指标。

## 6. 最小验收

| 编号 | 必须验证的结果 |
| --- | --- |
| RB1 | 改动 Runtime/Run 中的业务字段后发布 checkpoint，业务表、Operation 和领域事件不变；未提交内容不因重启成为业务事实，返回视图不通过可写别名污染 actor |
| RB2 | 与原生命令并发发布 checkpoint 不覆盖新答案，checkpoint 发布自身不推进业务控制版本；查询与重启从权威记录和同一快照构建一致视图 |
| RB3 | 三种模式的受影响恢复路径继续核对原身份与 Operation；未知结果不重复派发、不换会话，重复消息不漏处理或重复推进 |
| RB4 | 清理只作用于原 Execution 持有的精确资源；关闭回执未知仍保留记录；清理失败不撤销已完成的唯一 TaskDelivery 或重复解锁后继 |
| RB5 | 隐式差异写回及所有生产/测试依赖已删除；每类剩余写入均能定位到显式事务入口；heartbeat/checkpoint 无业务表副作用 |
| RB6 | 后端接口与真实调用方对应；平台协议解析位于适配层，纯业务规则无平台/存储依赖，unsupported 仍明确拒绝 |

扩展现有 `TestRuntimeViewCannotWriteBusinessState` 与并发答案/版本测试覆盖 RB1–RB2；从 app-server、原生 Orca、受监督 Orca 的现有恢复 fixture 中各选受影响路径，并复用清理/交付测试覆盖 RB3–RB4。只为缺失的 checkpoint 副作用和别名边界补充测试，不重跑所有平台故障组合。

以定向测试、必要的受影响包编译/依赖检查和写入调用点审查验收。默认不跑全量；出现具体跨模块失败或未解决风险才扩大范围，确需全量时使用现有并行入口。纯文档/清单变更只做链接与语义检查。PR 记录真实运行结果，不将模拟平台测试写成真机认证。

## 7. 系列完成条件

PR 描述附最终写入入口与后端职责映射，说明 PR 2 剩余项全部如何关闭。三项高优先级审计意见最终对应：

| 审计项 | 完成证据 |
| --- | --- |
| CommandService 抽象与 SQLite 实现不一致 | 已合并 #136/#137 的具体存储与 typed transaction 边界保持；生产代码不重新引入 SQLite 类型下转 |
| Runtime 混合读视图与隐式业务写入 | PR 2 的显式提交覆盖正常 worker 流程；本 PR 完成剩余写者迁移、读写数据分离和隐式入口删除 |
| 中心生命周期混入平台协议 | PR 1 提供事实边界；本 PR 完成实际后端接口及依赖收口，中心只处理业务关联、转换和副作用编排 |

达到以上条件后，本轮架构审计的高优先级建议完成。不以顺带完成新的 UI、多候选、性能优化或既有未认证平台能力作为本系列门槛。

## #156 基础实施记录（2026-09-30）

本项承接已合入的 [PR 2](architecture-02-worker-commit.spec.md)，按用户要求在同一个后续 PR 收完剩余写回与结构清理，不保留 Runtime 隐式写回的过渡入口。

| 写入归属 | 最终入口 |
| --- | --- |
| 固定 worker 生命周期、恢复、终态诊断和精确资源清理回执 | `Service.commitWorker` → `Store.commitWorker` → `SQLiteStore.commitWorker` |
| 历史 worker 活动事件 | `Service.commitEvents` → 同一显式事务 |
| 远端 Delivery ack 意图与回执 | `Service.commitOperation` → 同一显式事务 |
| 原生 Task、领取/交付、答案接收、调度控制与清理结果 | 现有 `CommandService` 类型化事务 |
| 服务控制恢复元数据与 checkpoint | `Service.publishCheckpoint` → `SaveCheckpoint` |
| heartbeat | `Store.Heartbeat`，仅更新已有 projection |

本地恢复将退休旧 Attempt、新 Attempt 和 Execution 当前代次与 checkpoint 原子提交；恢复核实结果也直接持久化，不依赖后续服务发布。

Service 私有 `domain` / `committed` 与公开 Runtime 通过深复制隔离。公开业务字段及未提交候选不能经 checkpoint 落盘，重启验证先初始化私有快照，再检查固定执行配置。

原生 Orca 的工作区/agent/coordinator 准备收在 `orca_worker_prepare.go`，Task/Dispatch 和反馈收在 `orca_worker_dispatch.go`，inbox/输出/轮次结算留在 `orca_worker_observation.go`，精确终端关闭与交付后清理收在既有 terminal 文件。共享 Operation 查询和问题结构分别移入 operation/decision，消除跨模式的命名和类型归属。

`ExecutionBackend` 移除无生产消费者的 ReadOutput、DetachObservation、ReconcileOperation 要求；输出读取保留在具体后端及协议适配层，移除无调用方的通用恢复/脱离转发方法。明确 unsupported 行为继续保留。

Windows 的服务连接检查先解析 checkpoint 中的仓库路径，避免同一仓库的 8.3 短路径和完整路径导致原生反馈被错误拒绝；固定仓库、remote、branch、claimant 校验保留。

### 整合主干后的验证

基线为 main@a05f797（含 #150、#151、#152）。执行了：

- 项目快速入口：go run ./cmd/test-feedback -quick，通过，准备 0.7 秒、执行 15.7 秒。
- go vet ./internal/steerline/... 与 git diff --check，通过。
- worker 提交声明/回滚、问题及答案不可变、回执失败不注册观察者、Task 终态保护、公开视图别名隔离、恢复代次原子切换、Orca 重启与精确关闭、同步及异步 app-server 两轮、原生问题回复恢复等针对性验证。
- 因为整合同时涉及服务恢复、持久化、原生交付和三种适配路径，扩大到项目提供的三进程入口：go run ./cmd/test-feedback -workers 3，准备 4.9 秒、执行 237.1 秒。

该次全量结果为失败：核心顶层 269 通过、19 跳过、12 失败；其他包顶层 132 通过、1 失败。其中两个 fixture 仍使用旧单 Attempt 错误文案/未提交的答案构造，已改为检查当前代次关联、通过真实 CommandService 接收答案，并定向重跑。没有增加测试跳过或把全量失败改写为通过。

剩余 11 个失败均在未修改的 main@a05f797 独立重现：

- TestConnectionClassifiesProcessAndSnapshotIndependently
- TestLifecycleReviewMultipleLiveAttemptsRefuseLaunch
- TestLifecycleReviewOfflineWithHeldLockCannotRestart
- TestWindowsPendingStartupPreservesMismatchClassification（config 子例）
- TestLifecycleReviewChildPublishesIdentityBeforePreflight
- TestLifecycleReviewExistingServiceDoesNotNeedInstalledBinary
- TestLifecycleReviewStopReportsExitWithoutReceipt
- TestNativeStoreRejectsRetiredSQLiteRunProjectionBeforeAuthorityOpen
- TestStartServiceReobservesExistingPendingAttempt
- TestStartupIDAndOutputPathValidation
- cmd/steerline 的 TestWaitServiceCommandResolvesIdentityAndKeepsPolling（Windows 临时 SQLite 文件清理时仍被占用）

原始日志与 summary 保留在本机临时目录 steerline-workerflow-integrated-full-20260930、steerline-workerflow-integrated-quick-20260930；没有把日志或可执行文件加入仓库。未启用真机认证开关，模拟验证不等于真机验收。

## 最终收口补充（承接 #156）

实施基线为 `main@d809a45`。#156 已删除隐式 Runtime 写回、发布基线和剩余业务写入依赖，本次补齐以下边界：

| 验收项 | 最终实现与证据 |
| --- | --- |
| RB1–RB2 | `Service.runs` 保存私有工作投影；`projectState` 重新构造公开 Runtime/Run，嵌套记录和 Run 指针均不与 actor 共用。checkpoint 只接受 `ServiceCheckpoint`，并发答案从数据库权威记录读取。`TestRuntimeViewCannotWriteBusinessState` 覆盖发布前的指针隔离与发布后的业务表不变 |
| RB3 | 复用 #156 的原服务归属、恢复版本、原请求核验及旧/新 Attempt 原子切换。定向验证本地恢复与新会话授权、原生 Orca 改名终端恢复及受监督 Dispatch 原身份核验 |
| RB4 | `workerCommit.Binding` 和 `ExecutionOperation` 显式声明绑定及停止记录；`putExecutionBinding` / `putExecutionOperation` 与 Execution、事件、checkpoint 同事务提交。停止先提交原意图，未知结果保留原 Ref；回执提交失败整笔回滚 |
| RB5 | 生产无 `saveRuntime`、`SaveRuntime`、`Store.baseline`、`domainFingerprints` 或 Runtime.PendingEvents。底层绑定/操作建数 helper 移到测试夹具，生产写入只经命令或声明事务 |
| RB6 | `ExecutionProbe`、观察/工作区用 `ExecutionBackend` 与 `ExecutionStopper` 按消费者分开。删除无生产调用方的资源结算要求；不支持停止的模式在意图写入前明确拒绝。Codex 事件和问题载荷、Orca 消息身份及原生保存回执解析位于协议包 |

绑定验证固定 backend、protocol、agent、target、process、workspace 与 generation；停止记录验证原 Run/Attempt/Ref、请求摘要、前值及合法状态转换。未增加旧版本读取或迁移路径。

当前主干的业务写入由 `CommandService` 类型化事务和 `Service.commitDeclared` / `SQLiteStore.commitWorker` 承担；服务发布由 `publishCheckpoint` / `SaveCheckpoint` 承担，heartbeat 只更新 projection。以上入口完成本系列高优先级审计项的实现收口；合并后可将本项状态改为已交付。

本次在上述基线上验证：

- `TestWorkerBindingRollsBackWithItsCheckpoint`、`TestExecutionStopIntentAndUnknownReceiptUseOriginalBinding`、`TestRuntimeViewCannotWriteBusinessState`、`TestAppServerServiceTwoTurnsPersistNativeReceipts`：通过。
- `TestDeadWorkerRecoveryPreservesDecisionAndFreshAnswer`、`TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace`、原生 Orca 改名终端/原会话恢复、受监督原 Dispatch 与 unsupported 停止用例：通过。
- `go test ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/workerflow -count=1`：通过。
- `go run ./cmd/test-feedback -quick -out <临时目录>`：通过，准备 1.1 秒、执行 11.4 秒；报告位于本机临时目录 `steerline-pr3-final-quick`。
- `go test ./... -run '^$'`：所有包编译通过，不执行完整测试套件。本次未运行全量或真机认证。
