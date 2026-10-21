# Spec：Worker 停机对账与恢复一致性修复

2026-10-13 用户修订：完整移除单候选放弃功能（控制台、CLI、API、服务端及专用状态字段）；任务级丢弃和唯一交付保留。控制台任务提示只显示中文状态，并增加全部执行候选数量列。下文涉及单候选放弃或旧列表列布局的历史要求不再适用；当前操作以[执行控制台](../archive/design/execution-console.md)为准。

任务 ID：87；任务名称：Worker 停机对账与恢复一致性修复。

状态：已获用户授权由本聊天直接执行；固定启动版本和实际交付结果见专属验收。[草稿](../roadmap/drafts/worker-stop-recovery.md) · [Plan](worker-stop-recovery.plan.md) · [验收](../acceptance/worker-stop-recovery.md)。原会话规则沿用[现行契约](../archive/spec/completed/product-simplification-04-conversation-recovery.spec.md)。

## 1. Worker 声明停服行为

按用户确认的行为：停服不尝试优雅生成 final；本地 Codex 停止本服务受管执行，Orca 的原执行继续，中心只脱离观察。这是 worker 的生命周期契约，Service/恢复状态机不按 backend、protocol、PID 类型或原生会话名称分支。

在现有 worker 生命周期能力中增加明确的 `ServiceStopBehavior` 标识，两种值：

| 值 | Worker 停服动作 | 当前实现 |
| --- | --- | --- |
| `stop_execution` | 有界关闭观察并释放本 worker 的精确 owned 执行资源，核实停止；保留工作区、会话与原意图 | 本地 Codex |
| `continue_execution` | 仅关闭中心观察与本地连接；不停止、关闭、重派平台 worker | 本机 Orca |

标识表达预期行为，不证明实际存活/退出。由具体 worker 实现声明，随执行冻结为非用户可任意覆盖的 worker 能力；放入既有冻结执行方案，参与摘要与版本核验。重启使用原冻结值，不读取当前可变配置来猜测；缺失/非法值或实现与原值不一致时在副作用前拒绝，不能用 bool 零值隐式选择继续。不得把它与 `ResumeActiveObservation` 混用：能否重接观察和停服是否停止执行是两个不同能力。

新增字段提升受影响冻结/持久版本，按项目规则拒绝旧格式，不迁移/双写/回退。因此本次修复不承诺接管诊断现场 101 的旧构建数据；它的失败保留为定位证据。

## 2. 恢复只与 worker 生命周期打交道

复用现有生命周期端口，在 worker 层收口停机与恢复入口；无存活观察器时也能由原冻结上下文重建该入口。Service 只调用 worker 的停服/核实恢复能力、接收规范事实与原观察流，保留单写提交、Command/Decision 授权和业务结算。无需新增通用插件框架或第二套运行状态。

| 层 | 职责 |
| --- | --- |
| Service / 恢复应用 | 保存恢复门槛，调用 worker，核对通用执行/Attempt/Operation 及版本，原子提交事实与资格，消费有效人工授权 |
| Worker 生命周期 | 声明停服行为，执行该行为，重接/核验原执行和未结算意图，返回 running/question/completed/interrupted/unknown 所需规范事实 |
| 具体 worker 实现 / 资源 owner | Codex 的进程树、thread/turn/journal；Orca 的 runtime/incarnation/terminal/Dispatch/worker_done；原生回执及 owned 资源操作 |

原生绑定封套、具体 Operation request 和回执内容由 worker 实现解码。上层只校验不透明身份/输入摘要并按标准 Operation 结果提交，不新增 `if local_codex`、解析原 endpoint/thread 或直接调用 `WorkerTreeAbsent` 等实现路径。现有 app-server 专属恢复 probe/apply 的原生部分移入 worker 生命周期；后台探针不写业务库，Service 接受结果的通用身份/代次/事务屏障保留。

复用 `RecoveryFacts`/`LifecycleStateFacts` 表达实际 WorkerLive、WorkerAbsent、WritersAbsent、Completed、QuestionPending、OutcomeUncertain 及原会话可用性，避免另建事实体系。具体实现负责事实来源与正确性，worker 停服标识不能覆盖真实事实。

## 3. 停机与恢复规则

先禁止新业务输入并保存既有恢复门槛，收取已到达的必要回执，然后有界调用每个 worker 的停服入口；一个错误不跳过兄弟资源。在准备/启动阶段尚无观察器的 owned 资源也由原 worker owner 回收，晚到回执只登记/回收原资源。突发退出无法留最后回执时，新实例仍沿原绑定核实。

本地 worker 停服删除额外 `turn/interrupt` 和新 `stop_worker_observation` 意图，只有受管资源释放路径；Close 返回或父进程消失不单独证明写者树停止。实现核实精确 owned Job/进程组及所有旧写者，证据不足保持 unknown。进程退出不生成 completed/final/成功或 TaskDelivery。

Orca worker 停服只断开观察；正在执行的原轮次仍获原输入授权，可以在中心离线时产生进展或完成。中心重开不重发业务输入、不创建替代 terminal/Dispatch，也不将“服务停止”标为“worker 中断”。停服不影响候选业务状态；候选放弃/Task 丢弃仍走独立的原 stop execution 流程。

| Worker 返回的实际事实 | 上层恢复行为 |
| --- | --- |
| 原执行仍运行 | 重接原观察，同 Attempt/session/原轮次，不开始新轮次 |
| 原执行等待回答 | 恢复原问题及原投递事实；旧实例未发送答案仍需按已有规则重新确认，不自动回答 |
| 原轮次在停服前或停服期间可信完成 | 接回原结果并按既有结算/ack 规则处理；重复重开不重复结算或报告结果；不要求为已完成轮次点击继续 |
| 原执行和写者已停止，原会话可核实、无未知业务动作 | interrupted，展示手动继续；当前有效授权后才用新 Attempt/进程继续原会话 |
| 身份、写者、原结果或相关外部动作无法核实 | unverified，保留现场并只对账，不派替代 worker |

停服期间 completed 仅证明原轮次，是否推进业务流程/发布/交付由既有业务规则判断；不把 worker_done 或 final 直接当 TaskDelivery。恢复接回结果及持久化后的确认不是新业务输入，按现有原请求与提交屏障办理，不为取证补发 worker_done。

## 4. 未知操作、资格与答案

支持的当前格式若存在已保存 stop observation 意图，worker 实现按原精确身份及目标核实后返回标准结算证据；不删除、不忽略、不重发 RPC。旧格式因版本不支持则拒绝，不能通过填默认标识读入。Codex 原树已停止只可核实停止目标，不能证明 start/input/reply/push/PR/publication 未发生；相关业务未知仍沿原请求对账。

在 worker 事实与相关 Operation 结果提交后，复用 `ClassifyRecovery` 将 OutcomeUncertain 一起计算。Decision 展示、Command 受理和外部投递前核验同一恢复资格，不先展示 interrupted 再变 unverified。现有 first-wins、恢复版本、Command 幂等和换代事务保留；fresh answer 恰好消费一次，接管或实质资格变化保留旧答案并关联新确认。已有未知发送只对账原请求。

相同事实轮询不换状态/Decision/版本或重复活动；原代次迟到结果不改新 Attempt/兄弟候选。Query/console 复用已有事实/原因/动作字段，区分“中心离线但 worker 可能继续”“已中断可继续”“状态未核实”和“答案已保存未发送”。不能凭配置标识向用户断言实际 worker 活动。

## 5. 最小验证

[WSR-A1–A8](../acceptance/worker-stop-recovery.md) 覆盖标识冻结/边界、两种停服动作、真实事实优先、原结果/问题/未知动作、重复/晚到和兄弟隔离。两路径都做受控产品集成；local 真实正常停服/手动继续和 Orca 真实活动停服/停服期间完成各一条最小验证。复用现有测试组/driver，不建新 suite 或第二套状态机。共享 #84 接线有序集成；安装现场与长期组合验收另行安排。
