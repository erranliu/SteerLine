# Plan：worker 停服恢复与手动继续

目标：`WORKER-SERVICE-RECOVERY`。唯一执行依据：[Spec](worker-service-recovery.spec.md)。状态：实现候选，结果及剩余证据见[验证记录](../acceptance/worker-service-recovery.md)。实现基线：`472d61e`；保留 #144 平台拆分、#148 root 清理及 #149 Orca Dispatch 提交修复。

## 1. 固定恢复边界与最小平台证据

先核对 [worker 适配路由](../../internal/execution/worker_execution.go)、[恢复路由](../../internal/execution/worker_resume.go)、[固定上下文](../../internal/execution/worker_execution.go)和 [Service 生命周期](../../internal/execution/service_state.go)。启动及轮询中的恢复需要从“可能准备并派发”拆成“读取事实并提交恢复判断”和“用户授权后执行”。尤其处理当前 `processCommandsAndDecisions` 在启动核验前投递已保存答案、`resumeWorkspaceWorker` 补准备/首次派发及 Orca 恢复补 Dispatch 的路径。

在隔离临时目录用目标 Codex CLI 做一次最小协议探针：保存 thread/turn/原问题，终止 app-server，核实原 thread 的历史读取及 `thread/resume` 后的原问题状态；确认开启新 turn 所需的处理。复用为 SR3 的前置，不另做一套大规模协议验收。该探针不据旧 call ID 发答案，不改 Codex 传输层来伪造 turn 完成。

Orca 先读现有 `worker_done` relay、原终端核验、Run/Task/Dispatch/消息回执和隔离 fake 能力。只采用已有证据支持的恢复/创建操作；没有原会话恢复或安全新建证据的分支返回明确的不可继续原因，不自动走另一后端，也不操作现有 Orca 终端。

完成条件：两条 worker 路径对 Spec §2 的事实分类与可用操作有明确映射；明确哪些 API 只读、哪些写操作需要新的人工授权。后续代码用同一映射，避免把平台细节放回通用 worker-flow 逻辑。

## 2. 共享规则、运行记录与事务

在 [workerflow](../../internal/execution/model/state.go) 增加最小纯规则：由核实事实和手动授权决定接回观察、结算、待决策及允许继续。保留五主阶段；中断不是成功，不用进程退出伪造原生轮次终态。平台事实结构由 worker 边界承接，避免建立通用插件框架。

扩展 [Execution/Attempt 记录](../../internal/execution/model/execution.go)、Decision/Command 和对应 SQLite/投影：Execution 保存恢复门槛及版本，Decision 保存继续方式、原因和关联旧输入，Command 固定来源 Attempt/恢复版本及输入。重建进程或 agent 时分配新 worker Attempt；普通反馈仍使用现有 Attempt。旧 binding、Operation、日志和会话游标保留，当前 Attempt 指针与授权消费由单写事务提交。

检查 [执行 binding](../../internal/execution/execution_backend.go) 的不可变校验以及 [app-server 异步回执](../../internal/execution/adapters/codex/lifecycle.go)：新代次不能覆盖旧 `process.json`、binding 或回执；将进程/thread 回执按 Attempt 保存，并在发送新轮次前完成持久化屏障。会话浏览可读历史 Attempt；推进执行必须匹配当前 Attempt 和固定目标。

持久格式变化按项目规则提升受影响版本并保留旧现场，使用新格式测试夹具，不写旧数据迁移。用 SR1/SR2 的规则和事务测试验证首个手动命令生效、重复点击返回原事实、迟到事件不能改变新代次。

## 3. 接入停服和重开门槛

改造 [Service.shutdown 与启动顺序](../../internal/execution/service_state.go)、[恢复入口](../../internal/execution/worker_start.go)、[Command 处理](../../internal/execution/command_processor.go)及 [worker 观察](../../internal/execution/service_workers.go)。

正常停服先禁止新的执行输入并保存门槛，再调用 worker 生命周期操作；local 关闭拥有的 worker 进程，Orca 结束观察。所有退出操作有界，进程是否真停由适配器核验；服务结束与 worker 状态分别持久化。回收停服时仍在启动中的 worker，保留已经产生副作用但回执未落盘的固定意图，不能丢失其身份。

新服务首先为前一服务实例的未结算 Execution 建立门槛并核验，再允许决策投递路径工作。即使没有正常停服标记，或有已接受但未发送的旧答案，也不会自动开始工作。只读对账可确认旧操作已经发生，禁止为确认它而再次执行。所有定时重试复用此门槛，自动模式不能恢复这个任务；未受影响任务按既有资格处理。

存活 worker 重新绑定原观察器；已完成结果正常结算；已终止且可安全继续的执行创建恢复 Decision；未知执行显示原因并继续有界只读对账。待答原请求、旧答案和新人工授权按 Spec §5 处理；确认旧答案使用关联的恢复 Decision，避免被既有答案幂等键当成已经授权发送。实现一个共享的继续命令验证入口，再按固定 worker mode 路由，不能依赖可变的当前 Profile。

完成条件：SR1 通过，包括停服期间进程状态变化、服务反复重开和旧答案未发送；无需 worker 存活假设即可使服务就绪。

## 4. local Codex 适配

改造 [本地预检](../../internal/execution/adapters/codex/lifecycle.go)、[app-server 启动/重接](../../internal/execution/adapters/codex/lifecycle.go)、[恢复结果应用](../../internal/execution/adapters/codex/recovery.go)和 [问答投递](../../internal/execution/adapters/codex/ask.go)。原生证据解析放在 [codexworker](../../internal/execution/adapters/codex/outcome.go)，RPC 留在 `codexapp`。

为 worker 使用受服务管理的普通子进程启动与清理，优先复用现有平台进程管理代码；Windows 不请求完全 breakaway、不将 Job 内视作 worker 能力失败。保留 PID/创建时间/可执行文件身份以及子进程树归属，确保正常停服能关闭本次拥有的 worker。独立服务启动使用的 `startDetachedProcess` 保留严格隔离检查。Linux 本地实现采用相同产品语义。

恢复先区分原进程存活、已退出和无法核实。存活时重接原 endpoint/generation/thread/turn，只恢复观察。退出时从已保存日志和原生历史核实结果；成功结果按原轮次结算，活动残留记录说明中断且不合成成功事件。终止的旧请求置为不可投递，原问题及用户输入完整保留。

用户手动继续且条件满足时，创建新 Attempt/app-server；原 thread 可恢复则核实 thread/worktree 后绑定，显式选择新会话才允许 `thread/start`。恢复输入包括已中断事实、原任务契约、当前工作区及经用户确认的输入，使用新 Command/turn ID。`thread/resume` 的实际状态按步骤 1 探针处理；无法确定旧活动已结束时返回待决策，不能并发启动新 turn。

完成条件：SR2 的本地分支与 SR3 通过；当前 Windows Job 宿主上无需 SCM/管理员即可启动本地 worker，退出服务后重开不会自动继续。

## 5. Orca 适配与统一人工入口

在 [Orca worker 证据（历史）](https://github.com/erranliu/SteerLine/blob/872dba4462a4f659d7685eb2a397211a1653c632/internal/execution/adapters/orca/inventory.go)、[native 接线（历史）](https://github.com/erranliu/SteerLine/blob/872dba4462a4f659d7685eb2a397211a1653c632/internal/execution/adapters/orca/lifecycle_observation.go)、[结果 relay（历史）](https://github.com/erranliu/SteerLine/blob/872dba4462a4f659d7685eb2a397211a1653c632/internal/execution/adapters/orca/worker_resources.go)和 worker 恢复路由中对齐共享事实分类。native 与 supervised 各自保留原协议，不套用 Codex 的 thread/turn 或 PID 规则。

原 terminal/Dispatch 活动时接回观察；终端已退出但 `worker_done`/结算证据可信时仍结算原结果。runtime/incarnation 变化或资源缺失仅证明身份不可用时保持未知，不能直接重建。需要补报或重新派发等外部写操作的恢复分支由人工命令授权后执行，读到的既有回执可直接确认。

手动继续优先采用平台能够证明的原会话路径。原会话无法恢复且旧活动已排除时，只有“新会话继续”的明确选择才可在原 worktree 建立新的 agent/terminal/Attempt 与新 Task/Dispatch；保留旧身份和消息历史。平台缺少安全创建能力时禁用操作并显示原因，不因此重建 worktree 或把复用目录称作复用会话。supervised 未支持的创建/取消能力不在本任务补齐。

更新 [反馈路由](../../internal/execution/worker_execution.go)、[可恢复反馈](../../internal/execution/worker_execution.go)、Decision/Query DTO 与控制台：统一展示“原 worker 仍运行”“已中断，可手动继续”“状态未核实”；继续动作说明是原会话还是新会话，并展示准备带入的旧输入。使用同一 Command 通道、版本校验和固定执行配置。

完成条件：SR1/SR2 的 Orca fake 分支通过；保留终端标题变化和退出后结果结算的定向回归。不会启动或操作生产 Orca 终端，不把未验证的新平台能力标为可用。

## 6. 定向验证与运行文档

SR1/SR2 尽量合并到现有 worker-flow、app-server、Orca fake、Command/Decision 和控制台夹具，只增加本次行为需要的断言；覆盖创建/投递回执丢失的一个代表性故障点，不构建完整崩溃组合矩阵。SR3 使用临时仓库和独立状态，复用现有真实 Codex 服务测试辅助程序，改验“退出后等待手动继续”，旧独立存活测试不能再作为本机派发前提。

实现最终候选运行 `go run ./cmd/test-feedback -quick`，追加未被入口覆盖的 SR1/SR2 定向用例和 SR3 一条本机闭环。Windows 候选构建按真机需要执行；改动跨平台进程代码时增加 Linux 编译核验。已有定向测试通过后，仅在新改动或具体风险出现时扩大范围，默认不跑全量。

更新 [worker-flow](../archive/spec/completed/worker-flow.spec.md)、[本地 app-server](local-codex-app-server.spec.md)、[Orca native](worker-flow-orca-native.spec.md)的适用边界及配套 Plan，更新 [数据契约](../product-design/data-model.md)、[决策说明](../product-design/decisions.md)、相关运行说明与 [执行架构共同约束](execution-architecture.spec.md)。保留旧验收的原始事实，注明新语义由本 Spec 替代，不把新编写的文档视为验收通过。

原始证据留本次新临时目录，最小结果在任务专属验收文档记录：代码/CLI 版本、测试方式、原/新 Attempt 与进程或 terminal 身份、thread/turn 或 Dispatch、原问题、用户继续命令和是否产生外部发送。验证目录和测试进程按归属清理；不访问 `C:\SteerlineRuntime`、#101、现有 Orca 终端，也不领取真实任务。

实施期间只对隔离夹具和临时进程做定向验证；旧独立存活验收仍作为历史证据，不替代本机新语义的验收。


## 测试计划（历史文档整理，非产品验收）

本次只修正文档导航及重构后的源码定位，不改变原实现或待补验收要求；原验证结果和限制见上述验证目录。

| 级别 / 组 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 文档静态 / doc-links | 当前仓库、Python 3 | `python scripts/check_doc_links.py` | 产品设计迁移及引用修正后 | 本地导航无缺失目标 | `docs/validation/product-design-layout-2026-10-07/README.md` |
| 文档静态 / task-identities | 当前 Git 仓库、Python 3 | `python scripts/roadmap_task_id.py check`；`git diff --check` | 交付前 | 任务 ID、名称和 Spec/Plan 一致，无空白错误 | 同上 |

不选择 L1/L2/L3、Full 或原生真机组：本次没有修改实现、测试场景、协议或平台接线，运行它们不能补齐文档导航证据；后续实际实现或补验仍按原 Spec 的范围选择。无需新增场景或测试环境能力。

## 测试计划（自动验收与当前待补）

任务专属记录：[停服恢复验收](../acceptance/worker-service-recovery.md)。SR1-A/SR2-A/SR3-A 映射原 Spec 的自动范围，历史已验只绑定原采样源码；另列 SR2-ORCA-A 的安全重建真机缺口。

| 验收项 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与登记 |
| --- | --- | --- | --- | --- | --- |
| SR1-A / SR2-A | 定向 Go / recovery owner | Windows、隔离模型/SQLite/受控 worker；`go run ./cmd/test-feedback -- ./internal/execution/model ./internal/execution -run '^Test(ClassifyRecovery|WorkerRecoveryAttemptCommitsAtomically|DeadWorkerRecoveryPreservesDecisionAndFreshAnswer|ServiceRecoveryGateRequiresTransactionalOwner|AppServerRecoveryDoesNotRaceInFlightLaunch|AppServerLateReplyCannotModifyTerminalTask)$' -count=1` | 恢复门槛/接线改变后；已有结果先核对覆盖 | 分类、原请求/新 Attempt、重复/迟到和事务门槛正确；逐项核对 Spec，必要未覆盖分支补最小表驱动用例，非零命中/无 skip | 默认新临时目录 summary.json；专属记录对应自动项 |
| SR3-A | 原生 opt-in / 本地恢复 | Windows、实际 Codex/认证、隔离 Git/SQLite；独立验收会话设置 STEERLINE_WORKER_RECOVERY_LIVE=1，`go test ./tests/integration -run '^TestLocalCodexManualRecoveryLive$' -count=1 -timeout=15m` | 实际本地恢复行为改变且旧证据不足时；不默认重跑 | 真机宿主退出、原进程消失、重开不新 turn，公开人工继续命令后原 thread/新 turn 与交付事实正确 | 临时原生身份/通知/命令；本任务记录，不以 fake 代替真机 |
| SR2-ORCA-A | L3 / recovery / recovery.orca-agent-rebuild（待新增） | Windows、真实本机 Orca/Codex、原工作区/owned 资源；`go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-agent-rebuild' '-out=<本次空临时目录>'` | 安全创建能力、最小驱动及场景登记完成后 | 排除旧写者后才依明确新会话继续命令创建新 agent/Attempt；原 workspace 保留，无第二未结算写者；无法核实则保持未知且操作禁用 | 临时场景报告/原 terminal/Dispatch/退出与创建回执；专属记录 SR2-ORCA-A |

补齐安排：在原 native/driver/testenv 扩展最小 owned 资源停止/排除旧写者/创建与回执观察，先核对平台真实安全能力；新 live 场景登记 orca-codex chain、独立现场与清理，不用已有 recovery.orca-codex 重接证据冒充安全新建。能力缺失保持阻塞，不要求生产安装或自动提权。原生参数/环境覆盖执行后恢复，只清理本次精确资源。

必需自动验收完成后逐项写入专属文档（源码/摘要、命令、日期、结果/证据）并同步 execution-map。产品“人工继续”命令可由自动驱动调用，不等于本任务主要由人工验收；现阶段不另加人工等待。默认不选 Full、完整 L2 或无关 live，构建/vet/L1 只按实际接线影响选择；纯 Plan 修订不启动产品入口。
