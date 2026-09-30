# Plan：worker 停服恢复与手动继续

目标：`WORKER-SERVICE-RECOVERY`。唯一执行依据：[Spec](worker-service-recovery.spec.md)。状态：实现候选，结果及剩余证据见[验证记录](../validation/worker-service-recovery/README.md)。实现基线：`472d61e`；保留 #144 平台拆分、#148 root 清理及 #149 Orca Dispatch 提交修复。

## 1. 固定恢复边界与最小平台证据

先核对 [worker 适配路由](../../internal/steerline/worker_flow_adapter.go)、[恢复路由](../../internal/steerline/worker_flow_resume_adapter.go)、[固定上下文](../../internal/steerline/worker_flow_context.go)和 [Service 生命周期](../../internal/steerline/service.go)。启动及轮询中的恢复需要从“可能准备并派发”拆成“读取事实并提交恢复判断”和“用户授权后执行”。尤其处理当前 `processCommandsAndDecisions` 在启动核验前投递已保存答案、`resumeWorkspaceWorker` 补准备/首次派发及 Orca 恢复补 Dispatch 的路径。

在隔离临时目录用目标 Codex CLI 做一次最小协议探针：保存 thread/turn/原问题，终止 app-server，核实原 thread 的历史读取及 `thread/resume` 后的原问题状态；确认开启新 turn 所需的处理。复用为 SR3 的前置，不另做一套大规模协议验收。该探针不据旧 call ID 发答案，不改 Codex 传输层来伪造 turn 完成。

Orca 先读现有 `worker_done` relay、原终端核验、Run/Task/Dispatch/消息回执和隔离 fake 能力。只采用已有证据支持的恢复/创建操作；没有原会话恢复或安全新建证据的分支返回明确的不可继续原因，不自动走另一后端，也不操作现有 Orca 终端。

完成条件：两条 worker 路径对 Spec §2 的事实分类与可用操作有明确映射；明确哪些 API 只读、哪些写操作需要新的人工授权。后续代码用同一映射，避免把平台细节放回通用 worker-flow 逻辑。

## 2. 共享规则、运行记录与事务

在 [workerflow](../../internal/steerline/workerflow/state.go) 增加最小纯规则：由核实事实和手动授权决定接回观察、结算、待决策及允许继续。保留五主阶段；中断不是成功，不用进程退出伪造原生轮次终态。平台事实结构由 worker 边界承接，避免建立通用插件框架。

扩展 [Execution/Attempt 记录](../../internal/steerline/domain.go)、Decision/Command 和对应 SQLite/投影：Execution 保存恢复门槛及版本，Decision 保存继续方式、原因和关联旧输入，Command 固定来源 Attempt/恢复版本及输入。重建进程或 agent 时分配新 worker Attempt；普通反馈仍使用现有 Attempt。旧 binding、Operation、日志和会话游标保留，当前 Attempt 指针与授权消费由单写事务提交。

检查 [执行 binding](../../internal/steerline/execution_backend.go) 的不可变校验以及 [app-server 异步回执](../../internal/steerline/worker_flow_app_server_async.go)：新代次不能覆盖旧 `process.json`、binding 或回执；将进程/thread 回执按 Attempt 保存，并在发送新轮次前完成持久化屏障。会话浏览可读历史 Attempt；推进执行必须匹配当前 Attempt 和固定目标。

持久格式变化按项目规则提升受影响版本并保留旧现场，使用新格式测试夹具，不写旧数据迁移。用 SR1/SR2 的规则和事务测试验证首个手动命令生效、重复点击返回原事实、迟到事件不能改变新代次。

## 3. 接入停服和重开门槛

改造 [Service.shutdown 与启动顺序](../../internal/steerline/service.go)、[恢复入口](../../internal/steerline/worker_flow_service.go)、[Command 处理](../../internal/steerline/command_processor.go)及 [worker 观察](../../internal/steerline/service_workers.go)。

正常停服先禁止新的执行输入并保存门槛，再调用 worker 生命周期操作；local 关闭拥有的 worker 进程，Orca 结束观察。所有退出操作有界，进程是否真停由适配器核验；服务结束与 worker 状态分别持久化。回收停服时仍在启动中的 worker，保留已经产生副作用但回执未落盘的固定意图，不能丢失其身份。

新服务首先为前一服务实例的未结算 Execution 建立门槛并核验，再允许决策投递路径工作。即使没有正常停服标记，或有已接受但未发送的旧答案，也不会自动开始工作。只读对账可确认旧操作已经发生，禁止为确认它而再次执行。所有定时重试复用此门槛，自动模式不能恢复这个任务；未受影响任务按既有资格处理。

存活 worker 重新绑定原观察器；已完成结果正常结算；已终止且可安全继续的执行创建恢复 Decision；未知执行显示原因并继续有界只读对账。待答原请求、旧答案和新人工授权按 Spec §5 处理；确认旧答案使用关联的恢复 Decision，避免被既有答案幂等键当成已经授权发送。实现一个共享的继续命令验证入口，再按固定 worker mode 路由，不能依赖可变的当前 Profile。

完成条件：SR1 通过，包括停服期间进程状态变化、服务反复重开和旧答案未发送；无需 worker 存活假设即可使服务就绪。

## 4. local Codex 适配

改造 [本地预检](../../internal/steerline/worker_flow_local.go)、[app-server 启动/重接](../../internal/steerline/worker_flow_app_server.go)、[恢复结果应用](../../internal/steerline/worker_flow_app_server_recovery.go)和 [问答投递](../../internal/steerline/worker_flow_app_server_reply.go)。原生证据解析放在 [codexworker](../../internal/steerline/codexworker/outcome.go)，RPC 留在 `codexapp`。

为 worker 使用受服务管理的普通子进程启动与清理，优先复用现有平台进程管理代码；Windows 不请求完全 breakaway、不将 Job 内视作 worker 能力失败。保留 PID/创建时间/可执行文件身份以及子进程树归属，确保正常停服能关闭本次拥有的 worker。独立服务启动使用的 `startDetachedProcess` 保留严格隔离检查。Linux 本地实现采用相同产品语义。

恢复先区分原进程存活、已退出和无法核实。存活时重接原 endpoint/generation/thread/turn，只恢复观察。退出时从已保存日志和原生历史核实结果；成功结果按原轮次结算，活动残留记录说明中断且不合成成功事件。终止的旧请求置为不可投递，原问题及用户输入完整保留。

用户手动继续且条件满足时，创建新 Attempt/app-server；原 thread 可恢复则核实 thread/worktree 后绑定，显式选择新会话才允许 `thread/start`。恢复输入包括已中断事实、原任务契约、当前工作区及经用户确认的输入，使用新 Command/turn ID。`thread/resume` 的实际状态按步骤 1 探针处理；无法确定旧活动已结束时返回待决策，不能并发启动新 turn。

完成条件：SR2 的本地分支与 SR3 通过；当前 Windows Job 宿主上无需 SCM/管理员即可启动本地 worker，退出服务后重开不会自动继续。

## 5. Orca 适配与统一人工入口

在 [Orca worker 证据](../../internal/steerline/orcaworker/inventory.go)、[native 接线](../../internal/steerline/worker_flow_orca_native.go)、[结果 relay](../../internal/steerline/worker_flow_orca_relay.go)和 worker 恢复路由中对齐共享事实分类。native 与 supervised 各自保留原协议，不套用 Codex 的 thread/turn 或 PID 规则。

原 terminal/Dispatch 活动时接回观察；终端已退出但 `worker_done`/结算证据可信时仍结算原结果。runtime/incarnation 变化或资源缺失仅证明身份不可用时保持未知，不能直接重建。需要补报或重新派发等外部写操作的恢复分支由人工命令授权后执行，读到的既有回执可直接确认。

手动继续优先采用平台能够证明的原会话路径。原会话无法恢复且旧活动已排除时，只有“新会话继续”的明确选择才可在原 worktree 建立新的 agent/terminal/Attempt 与新 Task/Dispatch；保留旧身份和消息历史。平台缺少安全创建能力时禁用操作并显示原因，不因此重建 worktree 或把复用目录称作复用会话。supervised 未支持的创建/取消能力不在本任务补齐。

更新 [反馈路由](../../internal/steerline/worker_flow_feedback_route.go)、[可恢复反馈](../../internal/steerline/worker_flow_feedback_resumable.go)、Decision/Query DTO 与控制台：统一展示“原 worker 仍运行”“已中断，可手动继续”“状态未核实”；继续动作说明是原会话还是新会话，并展示准备带入的旧输入。使用同一 Command 通道、版本校验和固定执行配置。

完成条件：SR1/SR2 的 Orca fake 分支通过；保留终端标题变化和退出后结果结算的定向回归。不会启动或操作生产 Orca 终端，不把未验证的新平台能力标为可用。

## 6. 定向验证与运行文档

SR1/SR2 尽量合并到现有 worker-flow、app-server、Orca fake、Command/Decision 和控制台夹具，只增加本次行为需要的断言；覆盖创建/投递回执丢失的一个代表性故障点，不构建完整崩溃组合矩阵。SR3 使用临时仓库和独立状态，复用现有真实 Codex 服务测试辅助程序，改验“退出后等待手动继续”，旧独立存活测试不能再作为本机派发前提。

实现最终候选运行 `go run ./cmd/test-feedback -quick`，追加未被入口覆盖的 SR1/SR2 定向用例和 SR3 一条本机闭环。Windows 候选构建按真机需要执行；改动跨平台进程代码时增加 Linux 编译核验。已有定向测试通过后，仅在新改动或具体风险出现时扩大范围，默认不跑全量。

更新 [worker-flow](worker-flow.spec.md)、[本地 app-server](local-codex-app-server.spec.md)、[Orca native](worker-flow-orca-native.spec.md)的适用边界及配套 Plan，更新 [数据契约](../data-contract.md)、[决策说明](../decisions.md)、相关运行说明与 [架构约束](../roadmap/architecture-constraints.md)。保留旧验收的原始事实，注明新语义由本 Spec 替代，不把新编写的文档视为验收通过。

证据记录在 `docs/validation/worker-service-recovery/`：代码/CLI 版本、测试方式、原/新 Attempt 与进程或 terminal 身份、thread/turn 或 Dispatch、原问题、用户继续命令和是否产生外部发送。验证目录和测试进程按归属清理；不访问 `C:\SteerlineRuntime`、#101、现有 Orca 终端，也不领取真实任务。

实施期间只对隔离夹具和临时进程做定向验证；旧独立存活验收仍作为历史证据，不替代本机新语义的验收。
