# Spec 03：最小状态机的分阶段远程闭环与 viewer 人工决策

任务 ID：6；任务名称：分阶段远程闭环。

## 有人值守 MVP 交付边界

2026-09-28 范围收口：允许将已实现并有真机五步证据的远程能力交付为有人值守 MVP，完整 Spec 验收继续开放。本节是当前启用规则；下方按日期记录的早期生产阻断结论保留其历史含义，不再将所有剩余验收一律作为 MVP 的启用条件。

- 固定本机中心与已认证的 Windows 台式机64g 目标（Orca 1.4.209/1.4.215），仅 native 任务、minimal-v1 五步、每任务单候选。由人显式选择命名方案启动、处理问题和审阅合并；本次方案固定 Codex / gpt-6-luna / xhigh。不同任务的现有人工并行能力不变，MVP 首次使用按单任务值守。
- 交付门槛：六项必需受监管能力的真实证据和内容哈希、当前连接/仓库/协调终端预检、项目步骤及工具可用、模型实际启动；保留原生 Task、Decision、Operation、执行身份、幂等恢复和精确 head 真实合并核验。能力报告只证明已测能力，项目配置不能冒充认证。主机基础能力可在同一已测目标复用，各项目仍分别核验仓库身份、工具与固定步骤。
- 允许交付人工值守的配置与固定构建；不增加绕过能力检查的开关，不改自动派发默认方案，不放开未知结果重派，不启用远程取消。
- 持续无人值守可靠性、新主机故障矩阵与 2 秒可见 viewer、精确 turn/final 及平台真实同句柄换代继续验收，不宣称已通过。#3/#4/#6 保持开放；#29/#32/#24 的完整交付前置不变。
- 部署到既有 home 时先核验无活动/未知执行，受控停服并保存状态备份，再替换二进制、增加项目命名方案和持久能力证据。重启后回读原任务与服务配置；不清库、不迁移旧 Beads 项目。

本节定义最小交付，本文第 8–9 节仍定义完整远程验收。后续扩大目标主机或无人值守承诺时完成对应剩余门槛。

2026-09-28 新目标补充：远端已改为 `台式机64g`（本机 Orca 1.4.209、远端 1.4.215）。[新主机验收记录](../validation/spec03-desktop64g-2026-09-28.md)保留 Smart App Control 首次阻断和后续用户授权关闭的不同试次；新主机的六项受监管能力与 SteerLine 仓库原生五步 Task 91024、绑定 `yes` 答案、固定 head 的隔离 PR #78 合并、五个 Dispatch release/exited 已通过。完整 Run 无人工补按 Enter。首次 Run 的远端 `gh` 未登录，publish 使用已连接 GitHub 能力创建 PR；用户完成机器准备后，新隔离 Task 已以固定 `gh pr create` 命令创建并合并 PR #85，原生五步、业务答案应用与释放再次通过。UAC 启用重启后观察到受控取消及用户批准/程序启动；Orca computer-use 无法读取安全桌面，用户另提供匹配的 Git for Windows 提示原图，当前目标 P10 可见分支通过。home 的故障矩阵和 2 秒 viewer 证据仍限定旧主机；新主机的长期可靠性及 #3 精确轮次/平台真实同句柄换代继续开放。

2026-09-28 后续：[本机可见控制台刷新样本](orca-probe/samples/spec03-viewer-follow-2026-09-28.json)发现默认当前 Attempt 被误保存为历史固定选择，修复后新 Attempt 的 SQLite `created_at` 至 Orca 可见终端 `source=screen` 帧出现的保守上界为 1.118 秒。手动选择历史 Attempt 仍保持固定。此本机样本本身没有覆盖 home 远端业务消息，也不能替代 Windows 管理员/UAC 桌面截图；home 远端时延由下段独立验收；当前主机的后续 UAC 证据见上段。

同日后续：[home 业务消息到可见 viewer 的真机验收](../validation/spec03-viewer-latency-2026-09-28.md)先发现顺序读取时原 Decision 展示耗时 2.476 秒；并行独立只读源并加快前台刷新后，新隔离任务的原 Decision 在 1.669 秒内出现在 Orca `source=screen` 帧，两次远端 review→publish 步骤帧分别为 1.414 与 1.798 秒。两条测试任务均完成五步、固定 head 的真实 PR 合并、中心 Task 结项和五个 Dispatch 释放，当前 1.4.209/1.4.210 `SteerlineTest` 目标的 2 秒 viewer 门槛通过。生产项目认证与 Windows 管理员/UAC 捕获仍单独开放。

同日进一步完成[SteerLine 仓库在 home 的六项受监管能力认证](../validation/spec03-production-project-capability-2026-09-28.md)：在新 clone/工作树上只读核验并保存内容哈希报告，正常 `worker-start --agent` 试次完成 status、worker_done、精确 transcript 与 release。复用已有终端的独立试次不能结算 worker_done，已明确失败并精确停止，不能外推为受支持路径。此认证未在 SteerLine 仓库上运行原生五步任务或提交 PR；生产启用仍受 UAC、故障矩阵和本项目原生演练门槛约束。

同日[R07 implement 完成窗口验收](../validation/spec03-r07-implement-2026-09-28.md)在中心停服后保留原 Dispatch；原 status/worker_done 于离线窗口产生，同一 SQLite 重开后只将原 implement Attempt 对账为 succeeded 并进入 verify，最后唯一测试 PR #20 固定 head 合并、Task 关闭、六个 Dispatch 释放。verify 首轮结果格式失败，以及 verify 两轮和 review 共三次 `input_accepted` 未形成 turn 后需人工 Enter 的事实另行保留；该矩阵分支通过，不宣称无人值守生产可靠性。



2026-09-28 增补：[远端原生业务问答至真实合并](../validation/spec03-native-question-2026-09-28.md)在新的隔离任务 91024 上证明 `yes/no` 问题、中心答案、原 Dispatch 上的独立应用证明、五步验证、固定 head 的 [测试 PR #17](https://github.com/erranliu/SteerlineTest/pull/17) 合并及原中心 Task close。该运行的 publish 提示曾暂记 unknown，之后同一 SQLite 对账原 worker 结果，无第二个 PR。当前目标的业务问答主路径因此有原生任务证据；viewer 2 秒目标当时尚未通过，后续实测见上段；管理员/UAC 设置提示及其余故障矩阵仍开放。

2026-09-27 新增[远端原生任务与 DAG 真机验收](../validation/spec03-native-dag-2026-09-27.md)：隔离任务 103 已在 home 依次完成五步、发布测试 PR #16，并由原中心按固定 HEAD 核验真实合并、写入 Delivery 和解锁后继 104。当前 1.4.209/1.4.210 目标的六项必需受监管能力已有新的内容哈希证据；review/publish 的原 Dispatch 在中心暂记 unknown 后经同 SQLite 重启对账成功，全部五个终端精确 release。宿主 on-request 命令的一次性权限批准已由远端可见界面验证；后续[活跃 home 回复限额样本](orca-probe/samples/p10-home-live-reply-size-2026-09-27.json)又验证 8 KiB 发送前拒绝和同一问题正常回复。管理员/UAC 设置提示仍开放。此前 [P10 home 权限样本](orca-probe/samples/p10-host-permission-2026-09-27.json)另记录 Codex `workspace-write` 沙箱内的 Orca CLI 无法读取 runtime 元数据并交付 `worker_done`，后续受监管适配器承接该交付路径。以下较早状态段落保留原验收当时的记录。

同日后续[可见服务关窗样本](orca-probe/samples/spec03-native-r07-visible-window-2026-09-27.json)已证明原生 prepare 在中心窗口关闭后按原 Dispatch 恢复并进入 implement；[另一次协调终端丢失](orca-probe/samples/spec03-native-review-coordinator-loss-2026-09-27.json)使该隔离任务在 review 后阻断，不能把这次运行记为新的完整交付。[可见 viewer 刷新样本](orca-probe/samples/spec03-visible-viewer-latency-2026-09-27.json)当时未证明 2 秒目标；后续实测见上段。

2026-09-26 补充验收：[R19 原中心发布 Delivery 丢失回执](orca-probe/samples/spec03-supervised-r19-retry2-publish-2026-09-26.json)在真实测试 PR 已创建后丢弃首次 publish status Delivery、未入库/ack 即重开同一 SQLite；原 Dispatch 重放该 Delivery 后只绑定唯一 [PR #12](https://github.com/erranliu/SteerlineTest/pull/12)，按固定 head 合并、关闭原来源任务且重复轮询幂等。[viewer 实时决策](orca-probe/samples/spec03-supervised-viewer-decision-live-2026-09-26.json)和[发布状态](orca-probe/samples/spec03-supervised-viewer-publish-live-2026-09-26.json)使用真实中心 SQLite 与同一 `View()` 渲染路径，分别在入库后 126 ms 和 276 ms 显示；可见终端端到端截图仍待补取。此前 R19 的一次[重放比较失败](orca-probe/samples/spec03-supervised-r19-first-loss-failure-2026-09-26.json)暴露结果文件末尾 LF 误比，已修复并有幂等单测；另一次[prepare 身份失败](orca-probe/samples/spec03-supervised-r19-prepare-identity-failure-2026-09-26.json)被中心正确阻断。原生任务来源的目标端契约、能力选择及远端 worktree 身份准备已接入同一适配器，但真实原生任务闭环尚未验收。生产能力报告仍未认证，Windows 可见服务关窗及 P10 管理员/UAC 交互回执继续开放。以下较早状态段落保留原验收当时的记录。

当前状态（2026-09-26）：远端默认 Windows sandbox 已实际执行只读工具命令；[原中心完整真机回执](orca-probe/samples/spec03-supervised-r01-original-retry-complete-2026-09-26.json)证明同一 SQLite Run 依次接收 prepare→implement→verify→review→publish_pr、原业务问答、唯一测试 [PR #8](https://github.com/erranliu/SteerlineTest/pull/8) 的精确发布与真实合并，并由重新打开的原中心关闭预置的隔离 Beads task，重复轮询幂等。R02 的真实 worker-start 回执丢失保持单一原 Dispatch 和 `outcome_unknown`；R07 verify、[review](orca-probe/samples/spec03-supervised-r07-review-staged-2026-09-26.json) 与 [publish 完成窗口](orca-probe/samples/spec03-supervised-r07-publish-complete-2026-09-26.json)均重接原 Dispatch，后两者继续完成测试 PR 合并与原来源任务结项；[R08 真实远端 runtime 替换](orca-probe/samples/spec03-supervised-r08-runtime-replacement-2026-09-26.json)使旧 binding 的观察返回身份不匹配，同时原 worker 仍以原 question/Dispatch 结算；[R16 真机修复循环](orca-probe/samples/spec03-supervised-r16-repair-cycle-2026-09-26.json)证明 v0 验证失败后由中心规划唯一修复、提交新候选并独立重验通过，另有 review `changes_requested` 与零预算阻断回执。历史 applied Decision 在较晚 Attempt 重启时的身份校验缺陷已修复。此前 [PR #5](https://github.com/erranliu/SteerlineTest/pull/5) 的首次 publish 因内层 task ID 错误被中心拒绝，后来只在重建投影中精确对账；原失败 Attempt 的生产恢复仍缺失。viewer 实际展示与延迟、R07 Windows 可见服务关窗、原生任务来源及 P10 Windows 管理员提示处理尚未完整验收；能力报告保持 `not_tested`，生产 claim 仍阻断。详细过程与残留见[Plan](03-remote-execution-and-viewer.plan.md)。下述 2026-09-25 状态段落保留为历史记录。

状态：部分实现，Spec 03 尚未验收。2026-09-25 的隔离真机 probe 验证了 1.4.209/1.4.210 版本族下的远端执行；受监管[prepare 回执](orca-probe/samples/spec03-supervised-pwsh7-direct-prepare-2026-09-25.json)确认启动、结构化结果、workspace/base、Delivery ack 和 release。[派发后](orca-probe/samples/spec03-supervised-center-restart-2026-09-25.json)、[status ack 后](orca-probe/samples/spec03-supervised-status-restart-2026-09-25.json)与[答案保存后](orca-probe/samples/spec03-supervised-answer-restart-2026-09-25.json)的中心重启验证精确重接原 Dispatch。[正常问答](orca-probe/samples/spec03-supervised-ask-reply-2026-09-25.json)、[reply 响应丢失](orca-probe/samples/spec03-supervised-reply-loss-2026-09-25.json)及[答案保存后离线](orca-probe/samples/spec03-supervised-answer-offline-2026-09-25.json)验证原 question/Attempt 绑定、平台回执或精确答案消息对账；PR5 新增的答案摘要应用证明仍待真机重验。[三阶段受监管回执](orca-probe/samples/spec03-supervised-staged-implement-verify-2026-09-25.json)证明同 worktree 的 prepare→implement→verify 独立派发、候选 commit 固定及真实 content-smoke/祖先检查回执。[PR #4 精确对账回执](orca-probe/samples/spec03-supervised-publish-pr-reconciled-2026-09-25.json)在由三阶段证据重建的独立临时中心投影中绑定唯一 Draft PR、已验证 head 与 main；随后真实 PR 按固定 head 合并，中心[合并与隔离 Beads 结项回执](orca-probe/samples/spec03-supervised-merged-task-close-2026-09-25.json)核验 merge commit 并关闭临时提供方 task。原发布失败 Attempt 的生产恢复、远端真实 Task 结项以及其余故障矩阵仍未满足完成定义。配套 [Plan](03-remote-execution-and-viewer.plan.md)保留失败探测与资源记录。

当前实现加强了 minimal-v1 verify 检查回执门槛、viewer 阶段展示，以及 `orca_supervised` 的显式配置与 claim 前远端身份/版本族/能力预检；受监管 minimal-v1 路径已有独立持久 Run-create Operation、worker 启动、精确 workspace 观察、结构化消息入库与 ack、worker 结算释放实现。prepare/implement/verify 回执、严格 schema、Windows 路径规范化、本地/Orca 身份检查及直接 PowerShell 7 发送已有代码与真机证据。正在运行的原 Dispatch 在中心重启后可按持久 start Operation 和 binding 精确重接 observer；prepare 的派发后、status ack 后重启已通过。单个远端业务问题的保存、回答和回执丢失只读对账已有真机回执；当前 PR5 应用证明门槛仍待重验。隔离重建投影还验证了精确既有 PR 绑定、真实合并回执和临时 Beads 结项；这些不代替原 Run 的失败发布恢复与真实来源 Task close。能力报告仍将 start/output/message/resource settlement 标记为未验证，scheduler 继续在 claim 前拒绝生产派发；review、生产 publish_pr 恢复、原生来源结项和完整 R01/R16/R19 验收仍未交付。Spec 01 的 P10 宿主权限分支及其余真机验收仍是启用门槛。执行记录见配套 Plan。

[负向 verify 真机回执](orca-probe/samples/spec03-supervised-negative-verify-retry-2026-09-25.json)现已证明固定内容检查实际 exit 7、Orca 接受失败检查的结构化 status/worker_done、中心只规划第 1 轮 implement 修复且不发布 PR；prepare、implement、verify 三个独立 Dispatch 均已 release/exited。此前 `run_required` 失败注入按用户授权关闭了精确测试终端并封存原 Dispatch。R16 的审查请求改动、修复预算耗尽仍未验收；这次回执不表示整项 R16 完成。

[预启动 TUI 宿主探测](orca-probe/samples/spec03-prelaunched-host-helper-cancelled-2026-09-25.json)确认复用已就绪终端可启动 assistant turn，但远端 Codex 工具的 helper 在执行前被 Windows 以 1223 拒绝，因而没有受接受的业务回执。`agentWait` 为 null，不能推断已出现可操作的权限提示；中心现在把显式 `agentWait` 或该 helper 错误显示为待人工核实，不自动回答或重派。P10 的真实权限弹窗处置与稳定消息路由仍是启用门槛。

开发前置：[Spec 01](01-orca-remote-protocol.md) 的真实远端证据与明确限制；[Spec 02](02-execution-model-and-adapter.md) 的模型、存储和执行接口。可先开发受限的远程适配器；生产启用仍须满足本 Spec 的剩余门槛。依据：[执行架构共同约束](execution-architecture.spec.md)。

与 [#11 测试夹具](test-feedback.spec.md)、[#12 旧路径清理](beads-path-removal.spec.md)、[#13 图规则拆包](taskgraph-modules.spec.md)共享 Service、配置、worker 上下文、Query/Command 及测试接线。独立远端协议样例可并行，共同修改须串行集成，后集成方核验双方接口并重跑受影响回归；不通过修改已创建任务的固定依赖协调合并。

## 1. 目标与范围

中心 SteerLine 领取一个业务任务，通过 Orca 在远端独立 worktree 中逐步执行 implement、verify、review、publish_pr，viewer 显示阶段、输出和人工问题；用户回答后同一权威步骤尝试继续，经中心核实各门槛、PR 合并及结项后任务完成。中心重启或网络暂断不会重复派发。

本阶段生产调度保持一个活动 Execution，运行 Spec 02 的 `minimal-v1`；每个步骤可以使用项目基础技能或脚本，由中心控制顺序和门槛。后续不同任务的人工并行启动由 [人工并行启动任务](manual-parallel-start.spec.md) 扩展；同任务仍只有一个活动 Execution。本阶段不实现任意流程/DAG 编辑器、多 agent 并行、移动端、多租户或公开 HTTP 服务。数据与命令接口沿用 Spec 02。

## 2. 配置与接入前置

新增显式的受监管远程配置，至少包含固定 connection ID、home Orca runtime/协调者绑定、environment ID、远端 repo selector、agent、固定 model/reasoning_effort、输出/状态轮询参数和能力报告版本。当前配置字段见[配置片段](orca-probe/samples/supervised-config.fragment.json)；它仍不能启用生产派发：worker start、消息、输出和结算已有受限实现与局部真机回执，完整能力认证仍未通过。本机 Orca terminal v2 与受监管远端是不同协议实现，不从前者的配置隐式切换。远端 worker-start 显式传递已固定的 model/reasoning_effort；若缺少必需值，领取前拒绝，不继承远端用户的 Orca 默认模型。

Codex CLI 版本作为执行证据记录，不作为默认兼容门槛：每个 worker 必须成功运行 `codex --version`，把非空实际输出放入受监管状态回执；配置 `expected_codex_version` 只有显式填写时才启用精确 pin，省略或留空不限制版本号。当前验收目标环境报告为 Codex CLI 0.157.1，版本号仍会随真实回执记录。

Windows 中心服务从 [PR 3 固定入口](pr-03-service-lifecycle.spec.md) 由用户独立打开可见服务窗口；viewer 退出不停止仍开着的中心服务，关窗则结束中心监督。本阶段不得让 viewer 在后台代启中心服务，也不得把关窗当成远端 worker 的停止证明。用户重新打开服务窗口后，中心按持久身份和 Operation 对账，再恢复消息读取和派发判断；Linux detached 路径按其平台能力运行。

首版生产远程配置固定 `remote_cancel_available=false`。预检和 viewer 必须显示远程取消不可用；不能展示可操作的取消按钮、自动调用 Orca worker-stop，或在失败后按远端 terminal handle 兜底关闭。用户需要中止活跃远端 worker 时，中心保留身份与资源占用，提示人工处理。正常结算后的 release 不受此限制。

Orca 版本兼容键按 Spec 01 及用户于 2026-09-25 选择的 `major.minor` 比较：当前 1.4.209 中心与 1.4.210 home 可在 1.4 版本族内验收；跨版本族在预检中明确拒绝。能力报告保存两端完整版本，版本族匹配不替代本 Spec 的能力门槛。首版适配器对结构化消息采用不高于已验证 8 KiB 的序列化大小上限，超限在发送前拒绝。宿主权限提示不自动回答，无法可靠分类时显示待人工核实，不伪造业务 ask 或执行成功。

配置目标是远端时，fallback 固定为 fail。任何创建前后的错误都不得改在中心执行。能力缺失、连接身份失效、仓库不可访问等问题在任务领取前尽可能发现；领取后发生的问题必须保留任务与执行记录。

远端仓库包含已提交的项目步骤入口和必要脚本，project_steps 满足 Spec 02 契约。派发载荷使用远端相对路径、版本化引用或完整内容；不能引用中心服务目录中的文件、中心私有 Git ref 或 task-source 数据库路径。旧的整段 start/finish 入口若不能在指定步骤停下，必须先拆分接入，不能直接用于 minimal-v1。

任务来源与结项的权威留在中心，只接受显式 native 来源。远端技能获得固定的原生任务内容与已领取事实，不调用 `bd claim/close`，不复制数据库。依赖旧 start/finish、Beads 或 completion 的入口在预检拒绝；接入项目须提供并验证符合当前 project_steps 契约的步骤入口。服务端不提供过渡 Beads 来源或旧结项适配。

原生项目复用 [原生接入 Spec B2](beads-exit.spec.md) 的任务上下文和本 Spec 的步骤/结果通道，由 [固定 DAG Spec D3](task-dag.spec.md) 的中心事务核验交付并关闭任务，不调用 Beads 或旧 completion。任务源不是后端类型，不能因接入远端自动切换权威；原生任务接入也不扩大本 Spec 的版本、消息大小或取消能力。

## 3. workspace 与派发

1. 中心完成配置预检，固定目标仓库/分支、技能协议版本及 base commit。WorkflowRun、Execution、StepAttempt 和任务领取意图按下一步与 claim 在同一中心事务提交。
2. 由中心领取任务并核实身份与领取者，之后才允许派发 agent。按固定 DAG Spec D3 事务保存 claim、运行和意图，不重复创建本步骤记录。claim 响应不确定时保持待核实并读取中心权威，不能直接再次 claim 或派发；被他人领取时记录冲突并结束本次准备，不改写对方领取。
3. 验证或通过 Orca 执行已验证的准备操作，使远端能够取得该固定 commit。远端不能解析中心私有 ref；分支名不能替代固定 SHA。
4. 保存派发 Operation 后，通过受监管 worker 创建远端独立 worktree，获取平台身份。Orca 返回的 ready 只证明启动协议到达相应阶段；取得运行/进度证据后再显示运行中。
5. worktree 分支、初始 HEAD、仓库身份和 base 必须在改动前核验。使用 Spec 01 验证的 Orca 元数据或远端确定性检查回执；不足时停止开发推进并标记准备失败，不能填入期望值冒充实际值。
6. 首个 worker 收到任务背景、implement 步骤契约和准备要求；准备检查失败不执行开发。agent 先通过确定性准备检查再开发，返回候选 commit 后结算本步骤，不自行进入验证、审查或发布。准备报告作为远端执行边界的证据，不宣称它是密码学证明。

中心收到有效 implement 结果后，先确认原步骤结算和资源状态，再在同一 Orca worktree 上派发 verify。后续 review、publish_pr 同理；每个步骤尝试都有自己的 Task/Dispatch 绑定和日志，前一步释放不能删除 worktree。首版允许每步新启动一个 agent 会话，输入携带必要的前置结果，无需靠模型对话记忆维护流程。

verify 使用固定的检查命令和清单，回传实际脚本回执；review 按配置执行且只报告审查结论。代码修复由中心重新派发 implement，并按 Spec 02 使旧门槛失效。直到同一 revision 的所有必需门槛通过，中心才创建 publish_pr 尝试。publish_pr 入口执行项目特殊 PR 流程时也受相同门槛约束；如果它需要改代码，必须报告 revision 变化并回到修复/验证，不能直接发布新代码。

每个操作和返回都绑定原 attempt。响应丢失而未获得平台 request ID 时保持 outcome_unknown，通过只读对账或明确人工恢复处理，绝不按 task 名称猜一个终端后重新发送。

## 4. 状态转换与观察

| 输入或证据 | 中心动作 |
| --- | --- |
| 派发意图持久化 | Attempt=dispatching；Execution=active；记录待核实 workspace |
| 平台接受并返回身份 | Attempt=dispatched；记录 Dispatch 与路由 |
| 同一尝试开始执行的证据 | Attempt=running；持续收集输出 |
| 有效 ask | 建立 Decision，Attempt/Execution=waiting_decision |
| 答案已保存 | Decision=answer_saved；生成 reply Operation，仍显示等待送达 |
| Orca 接受 reply | Decision=reply_accepted；不能据此宣称 agent 已继续 |
| 对应 decision_applied 或最终结果内的应用证明 | Decision=applied；未结算尝试恢复 running |
| implement/verify/review 的 worker_done 与门槛结果有效 | 结算当前 Attempt；按中心状态机生成下一步或修复意图，Execution 仍为 active |
| publish_pr 的成功 worker_done + 平台核验通过 | Attempt/Execution=succeeded，选用该结果，WorkflowRun=waiting_pr_merge |
| 明确失败报告或 review 请求改动 | 记录结果；确定性检查失败/审查改动请求按固定修复预算回到 implement，其他失败或预算耗尽进入 failed/blocked |
| 已确认退出且无有效结果 | Attempt=failed，Execution/WorkflowRun=blocked，记录 result_missing；不生成成功结果 |
| 网络中断或观察过期 | 更新 connection/liveness 及 freshness，保留已知业务阶段 |
| 支持 Stop 的本机适配器取得平台停止证据 | Attempt/Execution=stopped；记录谁请求以及停止结果；当前远程适配器不提供此操作 |

所有非法转换、身份不匹配和旧尝试消息保留诊断证据，不推进当前流程。输出文字、TUI idle、进程退出码或心跳单独都不能替代业务结果。

没有输出超过 stall_timeout 只产生需关注提示；有可能仍存活时，不因此自动杀进程或重派。agentWait 对象显示“宿主交互待处理”；null 表示本次检查没有等待证据，缺失显示未知。只有真实 Orca ask 才进入业务回答入口。

原生任务层按 [固定 DAG Spec D3](task-dag.spec.md) 将未合并 PR 的任务丢弃时，本远端适配器停止该任务的新步骤派发和正常结项；已派发的远端 worker 可能继续，保留精确身份、资源占用与观察/清理待办，不调用不受支持的远程取消。迟到结果只归入原执行历史，适配器不自行生成另一套任务状态。

## 5. 消息、输出与人工回答

中心独占本项目配置的协调 inbox 消费职责；viewer 不直接消费或 ack Orca 消息。使用有界 check 等待并行执行状态/输出轮询。默认状态与输出目标刷新间隔为 2 秒，单次网络操作有超时，不得因长等待阻塞命令提交或 viewer 查询。

按 Spec 01 的实际 Ack 语义持久化整个 Delivery，并处理所有消息。未回答问题需要保留原 Delivery 时，仍用已验证的观察/只读消息读取更新界面；新消息和完成事件不能因用户暂未回答而永久不可见。完整入库与业务应用分别记录，重放不重复追加规范事件。

worker-read 优先可用的 transcript，回退时明确标记 terminal。source/cursor 一起持久化；只有内容写入成功后推进 cursor。截断、历史淘汰和 source_changed 写入显式 gap 事件，不以“无输出”掩盖。展示输出不参与决定任务完成。

远端接入复用 [控制台 4A](pr-04a-execution-backends.spec.md) 与 [4B](pr-04b-conversation-events.spec.md) 的执行/会话契约及同一 UI，不新建远端专属消息模型。分别验证远端 agent/协议版本的 turn、phase、final、历史与等待能力；本机证据不能代验。精确折叠遵循 [4C](pr-04-console.spec.md)，缺能力采用明确的执行级展示并保留未验收项，不能因 worker_done 或 terminal 静默而制造 final。完整正文、晚到回复、source segment、去重和游标恢复沿用同一存储契约。

回答走 Spec 02 的 SubmitAnswer 命令：精确 decision ID、attempt、expected_version、选择/备注及幂等键。服务事务检查问题仍有效、答案符合请求、目标尝试仍权威，然后保存答案和 reply Operation。重复同一提交返回原结果；不同答案竞争时返回冲突并展示已接受答案。

服务离线时 viewer 只显示问题和恢复服务入口，不写答案；恢复后经同一 SubmitAnswer 命令提交。同一业务问题的回答不因重连再次生成新消息。若原问题或 Dispatch 已失效，答案记录保留并标记 failed/superseded，不发送给替代 worker。

业务自由文本答案与 Orca 终端按键输入分开。登录、信任目录和宿主权限提示展示明确处理原因；本阶段不通过 viewer 自动回答这些提示，也不把其处理后果伪装成业务决策已应用。

## 6. 结果、PR 与结项

按 Spec 01 接收与受接受 worker_done 对应的结构化结果，核验业务身份、平台身份、工作区事实和结果版本。缺失、冲突或超限结果进入 blocked/result_invalid，即使 Orca 已结算成功也不能宣告业务成功。

每个阶段结果按 step_id 校验各自 schema 和 input/output revision。实现提交的候选 commit、验证回执、审查结论及发布结果之间保存明确关联。通过门槛是中心的持久状态，不接受 agent 在最终总结中一次性自报前面阶段全部完成。

publish_pr 成功结果包含精确 PR URL、实现 head commit、目标分支与固定 base。中心向平台验证实例/仓库、PR 源与目标、结果 commit 对应关系及实际 merge 证据；不得按相似标题寻找 PR。必要的 Git commit 核验在中心仓库或平台可验证接口完成，不读取远端绝对路径。发布后的 PR head 变化需重新核验并记录，不能把旧 commit 上的测试/审计证据自动算到新 commit 上。

发布请求响应丢失时，查询精确仓库、源分支、目标分支及记录的发布身份，核实原 PR；不能使用模糊标题搜索，也不能直接新建第二个 PR。无法唯一确认时保持 outcome_unknown 等待处理。

等待 PR 合并时释放执行槽，保留任务领取、选用结果和 PR 绑定。worker 资源按 Orca 规则完成 release/明确的保留处理；释放 agent 终端不等于删除 worktree，远端产物默认保留，不触发公开发布。

PR 合并后，由中心按固定 DAG D3 提交 TaskDelivery 与 closed。部分失败继续 completing 并按原操作身份对账，不重复开发或跳过结项事实。PR 未合并关闭或绑定漂移则 blocked，不能关闭任务。

停止整个服务仅停止中心监督并保存状态；Windows 关服务窗口也会停止中心监督，远端 worker 可以继续，中心不得据此释放其活跃资源许可。当前生产远程取消不可用，相关命令返回明确的不支持并要求人工处理，不发送 Stop mutation。支持停止的本机适配器仍使用独立、持久化且可追踪的 Stop 操作。Windows 由用户重新打开固定服务窗口入口；启动恢复先处理已有消息、动作和执行身份，再考虑派发新任务。

## 7. viewer 与命令行

控制台通过 QueryService 和 CommandService 读取/操作当前格式记录；CLI 使用相同入口，不读取旧快照。选择 task ID 存在多个目标时必须要求 execution/decision ID，不能默认选第一项。

界面至少显示：

- 任务及 Execution 身份、agent、远端名称、实际 worktree/分支；任务行支持展开执行集合，即使首版只有一个。
- 当前步骤、每步通过/失败及证据对应的 commit、修复轮数、下一步或阻塞门槛；同时显示连接/liveness、最后成功观察时间和状态是否过期。
- activity 的来源、当前跟随状态、输出缺口；在重新连接期间仍可阅读已保存内容。
- 待回答问题列表、正文/选项/上下文、答案与回答者；已保存、平台已接受、agent 已应用分开展示。
- 启动不确定、宿主交互、结果缺失、结项失败等具体原因和现有可用恢复入口。不得提供实际尚未支持的恢复按钮。

命令提交返回 operation/command ID 和接受状态；提交按钮需抑制重复点击，但可靠去重必须由服务保证。viewer 重启后从持久记录恢复回答进度，不能依赖内存中的弹窗状态。

在真机验收环境测量消息到达中心后的展示延迟，目标为 2 秒内；记录网络/Orca 查询延迟，不对远端网络中断承诺实时状态。断线的判断依赖查询超时及新鲜度，不能仅凭 agent 沉默。

## 8. 故障验收

| 编号 | 注入点 | 通过条件 |
| --- | --- | --- |
| R01 | 正常分阶段派发、开发、提问、回答、验证/审查及发布测试 PR | viewer 展示各步骤和门槛，用户回答送往原尝试，远端无 SteerLine 服务 |
| R02 | Orca 已启动但中心未收到/未保存回执 | 恢复原请求或 outcome_unknown；不创建第二个 worker/worktree |
| R03 | 收到消息但本地事务未提交 | 重放后仍可处理，未提前 ack；只有一次有效转换 |
| R04 | 本地事务提交后、ack 前退出 | 重启去重，不重复决策/结果/结项 |
| R05 | 两个 viewer 回答同一问题 | 同答案幂等，不同答案仅一个生效，另一个看到冲突 |
| R06 | 答案保存后断线；reply 生效但回执丢失 | 保存待发送或不确定状态，按原请求核实，不盲目再次 reply |
| R07 | 任一步骤完成期间中心停止，包括 Windows 关闭服务窗口 | 用户重新打开固定入口后读取原结果并恢复正确的下一步骤；仅发布结果进入 PR 核验，不从头开发、不由 viewer 代启服务 |
| R08 | 网络中断、remote runtime 重启、身份替换 | 显示 unverifiable/身份变化；不操作替代终端，不把未验证退出算完成 |
| R09 | worker 退出却无结果；结果身份错误；重复 worker_done | blocked 或去重，不错误关闭任务 |
| R10 | 输出分页/source_changed/历史淘汰 | 已保留内容可读，缺失被标记，不影响持久业务消息 |
| R11 | 较早 attempt 的消息迟到 | 仅进入原尝试记录；不改变当前执行状态或回答目标 |
| R12 | PR 实际合并、PR 未合并关闭、结项部分成功 | 分别完成/阻塞/继续对账；task close 不重复产生副作用 |
| R13 | 派发引用中心本地路径，或项目仍依赖旧 start/finish、Beads、completion | 在预检拒绝，不以本机同名路径替代，不接入旧执行或结项路径 |
| R14 | Windows viewer 退出、服务窗口关闭与远程取消不可用 | viewer 退出而窗口仍开时中心继续监督；关窗只结束中心监督，不取消远端；远程取消入口明确不可用、不发送 Orca stop、不释放可能仍活跃的资源许可；本机 worker 按 PR 3 清理，本机 Stop 仍需确认退出证据 |
| R15 | claim 成功但回执丢失、领取者发生变化 | 中心回读核实后才能派发；不存在双重领取或对错误所有者启动执行 |
| R16 | 验证失败、审查请求改动、修复预算耗尽 | 前两者由中心安排有限修复并重新检查，耗尽后等待人工处理，期间不发布 PR |
| R17 | 验证通过后代码改变或旧 verify 结果迟到 | 发布门槛不通过；正确关联当前 revision/attempt，重跑已失效检查 |
| R18 | 同 worktree 从 implement 切到 verify/review/publish_pr | 每步有独立身份，无重叠修改；worktree 保留，故障恢复不串到旧步骤 |
| R19 | 特殊 PR 技能越步修改代码、提前返回 PR 或发布回执丢失 | 不越过门槛；标记越界/不确定并核实，未确认前不再次创建 PR |

协议替身和存储故障测试覆盖所有分支；真机至少执行 R01、R02、R06、R07、R08、R16、R18，并在测试托管仓库完成 PR 合并/结项路径。真机结果、版本及残留资源记录归档；无法实施的场景保持未验收，不能用本机替身替代。

## 9. 完成定义

远程配置有可复现样例；上述必需验收通过；现有原生 local/本机 Orca 行为回归通过；hosting、state-model、activity、decisions、skill-contract 和 data-contract 文档按实际实现更新。

完成后中心状态机实际控制准备、实现、验证、审查、发布、等待合并和结项；用户能在控制台看见各阶段、回答业务问题并恢复监督。项目基础技能/脚本只完成被派发步骤。并行 agent 与移动端不在本任务范围。
