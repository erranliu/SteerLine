# Plan Worker 三层职责重构

状态：已获用户授权实施（2026-10-14）。对应 [Spec](worker-three-layer.spec.md)与[专属验收](../acceptance/worker-three-layer.md)。本次执行结构重构与隔离验证，不运行安装或现场恢复。

## 输入与源码调查

调查基线 `1b7468b07f4fdcec31c59d2e3eb0d740816f4179`。已完整读取 `docs/spec/SpecGuide.md`，该基线 Git blob 为 `cb77f165b80f6e7bbb09fe190d0f86b97201e2c3`；依仓库开发规则作为普通开发编写，不执行产品 roadmap 转正/调度。已读取[长期流程](../product-design/worker-lifecycle.md)、[PR 契约](worker-pr-flow.spec.md)、[核心恢复](worker-disconnect-recovery.spec.md)、[停机恢复](worker-stop-recovery.spec.md)、[原会话边界](product-simplification-04-conversation-recovery.spec.md)、[屏幕核验移除记录](../acceptance/workflow-verification.md)及[测试入口](../../tests/e2e/README.md)。

实施基线：主干 `1b7468b07f4fdcec31c59d2e3eb0d740816f4179`，设计文档固定提交 `34418bd963af75b727850272dcffc1275690e99c`（本 Spec/Plan 路径）。随后用户补充的适用性要求纳入本次实施文档与实现；候选验证同时保存 runner source_commit 和工作树差异。保留他人改动，使用最新已核实基线实施，发现影响本范围的差异先更新调查与计划。

| 现有代码 | 调查结论与安排 |
| --- | --- |
| `worker_lifecycle.go`、`worker_execution.go` | 准备、执行意图、平台装配、回执处理混合；拆为中层规则、窄快照和 actor 映射，删除全层共享 workerFlowContext 的依赖 |
| `worker_environment*.go`、`worker_qualification.go`、`worker_recovery*.go` | 可靠性事实/资格与 Decision/步骤处理交织；前者归中层，后者通过通知与授权值回到应用/Workflow |
| `worker_events.go`、`worker_completion.go`、`worker_lifecycle_recovery.go` | 原生事件接受、原结果结算与后继同时存在；拆分原执行事实和业务结果路由，保留原子提交 |
| `staged_flow.go`、`staged_result.go`、阶段提示词 | 业务步骤、补报与报告来源保留并归 Workflow；技术状态读取改为规范执行事实 |
| `lifecycle_effect.go`、`lifecycle_receipt.go` | 保留异步 effect/receipt、原操作及绑定校验；算法归中层，队列/进程所有权及提交由 actor 组装 |
| `backend/lifecycle.go`、`backend/operation.go` | 复用已有动作、回执、身份和恢复能力；删除无消费者的 WorkflowPhase/TaskDelivered/TaskTitle 等泄漏值，按实际消费者收窄 |
| `adapters/codex`、`adapters/orcatransport` | 已有 Execute/Reconcile/Observe/Stop/Recovery，复用原生算法；通用推进归中层，必要原生复合准备保留 |
| `service_commit.go`、storage ports/SQLite | 保留 WorkerCommit 的声明写集合、CAS 和事务；只调整数据映射，不拆为跨层独立提交 |
| app 目录注册、product/query、testkit/driver | 更新构造及事实投影，公开 DTO 和操作保持；测试不增加产品后门或直接写业务 SQL |

生命周期契约家族已有约 34 个生产、41 个测试文件消费者；核心 worker 约 5,500 行。该统计用于安排整合，不是承诺改动行数。按职责集中实施；本任务不按三个层级拆成只能交付签名或半条路径的独立任务。

Rebase 核对：最新主干新增候选停止、启动保留及 Orca 项目仓库配对。候选停止/资源管理授权保持在应用侧，中层消费通用“禁止业务写入/允许原资源结算”约束；迟到回执仍补存原资源事实。Orca 仓库配对仍由 adapter 原生准备负责。G2 增加 CandidateStop/StartRetention/LifecycleEffect 回归，G3 增加仓库配对用例。

## 目标代码边界

| 位置 | 最小职责与消费者 |
| --- | --- |
| `internal/execution/workflow` | development/v3 的规则、输入构造/结果解释与后继；消费 ReliableExecution 的规范值，不调用平台或存储 |
| `internal/execution/reliable` | 执行快照、执行意图、变化/通知，准备/投递/观察/恢复规则；消费纯 adapter 契约，不接收 Service 或混合业务记录 |
| `internal/execution/backend` | 现有 adapter 能力、原动作和证据值；不新增通用插件框架 |
| `internal/execution` 根 | actor 驱动、命令授权、持久记录到窄值的转换、事务写集合、应用/PR 桥接及 effect 接线 |
| 原 adapter/app/storage owner | 原生实现、具体组装与现有提交；保持所有权 |

新的两个包是原规则的替代所有者，不是包装层。层间使用窄值，不为减少转换代码传入完整 Service、RunView、WorkflowRunRecord 或通用回调函数集。

ReliableExecution 采用现有非阻塞 actor 节奏：读取不可变执行快照与意图，计算下一动作/变化；异步 I/O 返回带固定原操作身份的事实；actor 接受事实后调用规则并组合一次提交。无需新 goroutine 调度系统或持久化引擎。

契约至少分开 `ExecutionIntent`、`ExecutionSnapshot`、执行变化/待执行动作、规范通知与有效授权。它们是本任务需实现的内部值，不是新增用户 API；命名可细化，职责不能合并为全层共享大结构。Workflow 的业务关联键传入中层时保持不透明。

## 实施步骤

### P0 固定所有权与行为基线

1. 按长期文档 W/E/V 梳理现有入口、状态写者、事务和消费者，列出每条旧路径的替代位置与删除条件。区分已用路径和仅保留的 preparation/probe 能力，不把后者变成新增启用行为。
2. 固定 actor、Workflow、中层、adapter 输入/输出及提交图，特别是结果接受、后继意图、Decision 与新 Attempt 的事务边界。
3. 选择下文既有测试和六个受控场景，核对非零匹配/登记/平台前置。新增层内测试与边界负例列入 P1/P2/P3；未有测试入口不得标验收通过。

### P1 建立真实层间契约

1. 创建 workflow/reliable 包及窄执行值；actor 从现有记录生成快照，可靠性包不导入混合 `execution/model`、reconstruction 或根 Service。
2. 窄授权表达当前执行/Attempt、原请求、授权/恢复版本和允许动作，正式 Command/Decision 的解析仍在应用侧；中层不能自行扩大权限或构造业务继续 prompt。
3. 实现 adapter 能力声明与原生事实的显式适用性枚举，零值按尚未核实处理。分别验证 Codex 原生 app-server/thread 流程和 Orca 原终端/启动/原生准备流程；本地不执行 Orca 专属 READY/连接核验。中层按声明选序列，not_applicable 跳机制、必需 unsupported 阻断、unverified 原目标取证/待处理。
4. 收窄 adapter Snapshot/Setup；通用计划逻辑与技术 budget 归中层，保留原生复合准备和原生材料 owner。检查序列化前后持久字段/摘要是否相同，不主动升版。
5. 在 `internal/testkit/boundary` 扩展依赖矩阵、传递依赖和负例：Workflow 直达 backend/adapter、ReliableExecution 依赖 workflow/混合业务模型/Service、adapter 回调中心写者都必须拒绝。中层导入 adapter 契约为允许边；不得扩大白名单放行循环依赖。结合层内 fixture 验证业务意图不能绕过中层执行资格，actor 只落实提交屏障与批准的原动作，不持有平行技术重试/恢复规则。

### P2 收口可靠执行并接入两条路径

1. 搬迁原准备、原操作对账、接受/开始/终态、期限、身份/旧写者、观察和恢复资格规则；中层返回规范事实与声明变化，不写业务阶段或 DecisionRecord。
2. 对 app-server 只有 endpoint/generation、尚无 thread 的部分绑定返回 unverified；不能因“已连接”开业务。已有同 thread 恢复 seed 仍按原授权准备。补适用性用例断言该部分成功不能进入 start_turn。
3. 本地 Codex 保留 owned 进程、initialize/thread/start/resume、原 turn 回执和历史；prepare_session 的部分 binding 与失败位置继续返回，首个 turn 前必要连接所有权保留。
4. Orca local/environment 共用同一 adapter，保留唯一原 terminal、启动接受状态及原生 main-agent 证据。已识别代理即 launch confirmed 的现有能力边界先保持，不顺便加入完整初始化取证或自动启动重试。
5. 重接、反馈、再核验和服务换代调用同一组中层规则。原生请求未知只 Reconcile，不由旧入口直接 Execute；异步效果不能跨 Attempt 改投当前目标。
6. 补 `TestReliablePreparationFacts`、`TestReliableInputOutcomeBoundaries`、`TestReliableRecoveryAndAuthorization` 和 `TestReliableLateFacts`，另补 `TestReliablePreparationApplicability` 和两 adapter 的适用性接线用例。使用纯 adapter fixture 验证部分成功/未知/冲突、明确拒绝与 accepted 未 started、授权版本、迟到/兄弟隔离；不模拟成功来省略守卫。

### P3 精简 Workflow 与恢复业务路由

1. 搬迁阶段输入、结果解析、report source、有限补报和业务反馈路由；删除对 native binding、原操作回执或恢复平台字段的读取。
2. Workflow 消费 prepared、turn ended、interaction/recovery required 等规范事实；因技术未知而暂停不生成新的业务输入。正式 continue 的新业务意图由 Workflow/应用产生，中层只校验和执行。
3. PR 查询/交付使用原 publication/任务 owner，不让中层认识 PR URL、merged 或 Task closed。Task 终态由 actor 传达“禁止新输入/仅结算原操作”的通用执行约束，不给中层完整 Task 记录。
4. 补 `TestWorkflowExecutionFacts` 与 `TestWorkflowSourceAndFormatRules`：用中层 fixture 验证三步路由、失败/未知暂停、源失败、报告格式次数、PR 复用和新输入授权；高层不配置模拟 terminal/thread。

### P4 保持单写与完整接线

1. actor 接受中层变化和 Workflow 后继，合成现有 WorkerCommit；同事务保存结果、步骤、下一输入、Decision/Operation/Attempt 关联，不调用两次独立保存。
2. 持久化意图确认后才执行外部动作；保存失败/旧 revision/Task 终态/服务换代拒绝。外部成功提交失败只重查原操作，原资源由已有 owner 结算。
3. 同步既有 controlled Orca worktree 响应的原 instanceId 字段，保持 create/show 同一身份；不省略产品原生身份守卫、不新增模拟开关。
4. 更新 `directory_local.go`、`directory_orca_terminal.go`、execution_service_wiring、查询投影及 fixture/集成/driver 的构造。UI/HTTP 只消费原 DTO，不建立新恢复入口。
5. 保留并迁移必要 commit boundary 用例，补 `TestThreeLayerAtomicCommit`：结果与下一意图一次提交、失败回滚、重放只结算一次、旧 Attempt 不能改变当前/兄弟执行。

### P5 删除旧路径并交付验证

1. 删除被替代 Service 可靠性/业务算法、旧 workerFlowContext 依赖、无用转发/类型别名/adapter planner；根目录只留下有明确用途的宿主、授权、记录转换和事务组装。
2. 源码核对新包确实拥有规则，根 Service 不能通过通用 closure/callback 把旧规则重新注入；adapter 不读取业务步骤或解码业务 JSON。
3. 执行测试计划，修复实际问题并按需要复验；结果登记 acceptance，不回写 Plan 为运行状态。
4. 受控恢复驱动只对明确的提交前 stale_recovery 拒绝有限刷新同一执行/Attempt 的当前合格决策（最多三次）；新请求记录其 ID，未知结果零换 ID 重投，不放宽产品版本门槛或场景期限。
5. 同步长期文档职责与当前缺口、源码架构、Spec/验收索引。原生行为/支持范围保持，初始化增强仍明确待实施。

## 测试计划

全部自动项 [TL-A01–TL-A07](../acceptance/worker-three-layer.md#自动验收项) 映射如下。G/R 是本文命令代号，非新增 E2E ID。D1 是文档静态检查；其余在实现后的指定时点执行。

| 命令 | 级别与组/场景 | 环境与时点 | 自动项 | 通过条件与证据登记 |
| --- | --- | --- | --- | --- |
| D1 | 文档静态检查 | 当前仓库；文档完成后 | 文档一致性，不算产品自动验收 | 链接/编号/映射/差异格式通过；登记专属文档的静态检查节 |
| G1 | L1 固定范围＋三层边界/规则定向 | Windows 隔离 fixture，无模型；P1–P3 后及最终 | TL-A01、TL-A02、TL-A07 | 包/符号/传递依赖负例有效；中层无 Workflow，Workflow 无原生知识；层内行为测试非零匹配、无失败/skip；summary.json 及用例日志登记对应项 |
| G2 | 定向提交、反馈/恢复、业务步骤回归 | 既有独立 Git/SQLite/受控进程；P2–P4 后 | TL-A03、TL-A05、TL-A06 | 原子、未知输入/无 final、原 source、授权与重放行为保持；记录具体用例及原发送计数 |
| G3 | 两 adapter 定向契约回归 | Codex 受控协议＋Orca local/environment fixture，无真机；P2/P4 后 | TL-A02、TL-A04、TL-A05 | 原生调用/参数与取证算法保持、部分 binding 不丢、未知零重发、两路接中层且无屏幕门槛；必要/跳过/不支持/未核实的能力组合有非零行为断言；登记两包用例日志 |
| R1 | L3 controlled：delivery.basic、recovery.waiting-merge-restart | runner 独立 home/Git/SQLite/loopback provider；P4/P5 后 | TL-A03、TL-A06、TL-A07 | 正式三阶段/PR/合并/唯一交付及依赖保持；原待合并身份不重派，closed 后查询守卫保持；summary/scenario.json |
| R2 | L3 controlled：recovery.codex-resume、recovery.report-source-changed | 同上，受控 app-server；P4/P5 后 | TL-A03、TL-A05、TL-A06 | 原 thread 新 Attempt 和业务实施/report 路由不重复；人工成果不冒充旧 source；原子/唯一交付保持 |
| R3 | L3 controlled：recovery.terminal-local-reconnect、recovery.terminal-environment-unknown-send | 现有 Orca 原生状态 fixture及受控 provider；P4/P5 后 | TL-A04、TL-A05、TL-A07 | local 原终端重接；environment 原请求未知仅对账，原始回复/结果一次结算，零新 terminal/Enter/输入 |

### 命令与入口补齐

```powershell
# D1
python scripts/check_doc_links.py
python scripts/roadmap_task_id.py check
git diff --check

# G1：P1-P3 新包和下列 Test 前缀在实施中落实
go run ./cmd/test-feedback -level=L1 -- ./internal/testkit/boundary ./internal/execution/workflow ./internal/execution/reliable -run '^(TestThreeLayer.*|TestWorkflow.*|TestReliable.*)$' -count=1

# G2：按下列三组定向执行，避免单个包默认 10 分钟总期限混合全部阶段回归
go run ./cmd/test-feedback -- ./internal/execution -run '^(TestThreeLayerAtomicCommit|TestWorkerFlow.*|TestWorkerPrompt.*|TestLifecycleEffect.*)$' -count=1 -timeout=10m
go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerFlowKeepsSessionThroughFeedbackAndPRChanges|TestWorkerCommit.*|TestWorkerReceiptFailure.*|TestAppServerFailedPreparation.*|TestWorkerIntentIsDurable.*|TestWorkerRecovery.*|TestWorkerService.*)$' -count=1 -timeout=25m
go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerStage.*|TestCandidateStop.*|TestCandidateStartRetention.*)$' -count=1 -timeout=25m

# G3：当前 adapter 入口的相关用例；故障行为不以单纯编译代替
go run ./cmd/test-feedback -- ./internal/execution/adapters/codex ./internal/execution/adapters/orcatransport -run '^(Test.*Lifecycle.*|Test.*Preparation.*|Test.*Recovery.*|Test.*Original.*|Test.*Unknown.*|TestAgentLaunch.*|TestOrcaNativeState.*|Test.*Repository.*|TestPreparationApplicability.*)$' -count=1

# R1-R3：全部 ID 已登记，使用现有场景而非新增完整矩阵
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled '-scenario=delivery.basic,recovery.waiting-merge-restart'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.codex-resume,recovery.report-source-changed'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.terminal-local-reconnect,recovery.terminal-environment-unknown-send'
```

G1 的三层负例和新包测试、G2 的组合提交测试是必需入口；不能运行零匹配后宣称通过。既有用例迁移时保留行为断言，更新真实包/测试名；实施验收前核对 G1–G3 各选定 owner 非零匹配、无失败/意外 skip。R1–R3 每个场景保持已有独立时限，不串入多个新增故障、不扩大期限；driver 如需接新内部类型只修改已有 native/evidence/driver owner。

若既有 R1 尚未断言 closed 后零 PR 查询，P4 在现有 provider fixture 增加本执行的查询计数，并在 R1 检查终态后的后续 poll/重启；不新增产品统计接口。缺失某项断言时必须补相应小型 fixture/用例，不用历史结果替代本次候选验证。

证据统一用 runner 默认临时目录或本次空的 `-out` 目录，包含 summary.json、选定用例日志、scenarios/<id>/scenario.json、必要身份/Operation/输入计数与 cleanup。在 [acceptance 自动项表](../acceptance/worker-three-layer.md#自动验收项)逐项登记实际路径、执行版本、分支及结论；一个场景可以覆盖多项。源码结构检查与原子断言均必需，集成通过不能掩盖分层失败。

不默认跑 Full、完整 L2、live、native 工作区删除、候选全矩阵或真实远端故障链：本任务不改原生调用/取证算法和公开业务行为，G3 与 R1–R3 覆盖实际接线风险。controlled 不证明真机能力；若实施不可避免改变原生协议、参数、状态来源或运行权限，先修订 Spec/测试计划并增加针对该变化的最小原生验证，不能默默扩大本次结构重构。临时/AI 调试必须先生成新 `C:\SteerlineDebug` 包；既有分级入口沿其隔离夹具，禁止安装环境和 merge 项目作为现场。

## 交付安排

完成代码整合、上述最小验证与结构自查后，更新专属验收明细和长期文档的实际实现归属，列明尚未新增的初始化能力。人工验收窗口：本次无强制项；公开行为改变时另行明确参与人、窗口及阻塞范围。

一个 Spec/Plan 对应一次完整三层重构交付。P0–P5 是内部实施检查点，不是三个可并行交付的 Task，不创建 roadmap 节点。用户已授权直接实施；本次不包含安装更新或任务 22 现场恢复。
