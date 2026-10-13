# Spec：Codex / Orca 核心断连恢复

日期：2026-10-12。状态：实现、审计修正及受影响最小自动验证已完成；live 全链未在本轮审计重跑，未部署。普通开发，无 Task ID，不修改 execution-map 或安装现场。实时证据见专属验收。

[Plan](worker-disconnect-recovery.plan.md) · [专属验收](../acceptance/worker-disconnect-recovery.md) · [L3 规划](../../tests/e2e/suites/recovery/README.md)

## 1. 审计结论与取舍

原规划不适合一次实施。它同时新增成果接纳流程、全阶段故障对账、恢复事务窗口矩阵、三种真实链和大量驱动能力，已接近一轮可靠性专项。身份、未知副作用、原结果来源等约束必须保留；过大的交付范围与重复的产品流程应收敛。

现有代码已有 RecoveryFacts、RecoveryVersion、原请求 journal、continue Decision、同 thread 新 Attempt、实施反馈及正常 report/publish 流程。首轮修复这些路径的缺口，不新建恢复框架或成果接纳子系统。

| 原规划能力 | 首轮取舍 | 理由 |
| --- | --- | --- |
| 原连接/终端重接、离线完成补读 | 保留 | 中心退出、短暂断网最常见；已有 adapter 路径 |
| 明确 failed/interrupted 无 final 的恢复入口 | 保留 | 直接解决失败汇报永久 reconciling |
| Codex app-server 退出后恢复原 thread | 保留 | 覆盖中心异常退出后原受管进程死亡 |
| 独立“核验并接纳现有成果”选项 | 删除独立流程 | 人工成果通过原会话新的实施轮次重新验证，复用现有 source 绑定 |
| Orca 丢失 terminal/runtime 后重建原会话 | 延后 | 当前 BindRecovery 明确拒绝替代终端，缺平台公开能力；首轮只提供明确原因 |
| 首次资源创建、原生问答、push/PR 回执丢失的全窗口增强 | 延后 | 属于不同副作用链；保留现有守卫及对账，不扩大首轮 |
| 全恢复提交窗口、远端真实故障链、硬件断电 | 延后 | 驱动成本高；首轮选重复确认、一个未知发送窗口和重复重启 |

基线是 `6d4b4fa2` 与尚未提交的 `u` 崩溃修复。源码审计依据：

- [worker_lifecycle.go](../../internal/execution/worker_lifecycle.go)：reconciling 的已停止轮次提前返回，可能跳过恢复资格与 Decision。
- [staged_flow.go](../../internal/execution/staged_flow.go)：非 implement 轮次先要求 final 来源，明确失败也可能留在 reconciling；已有反馈可回到 implement。
- [worker_recovery.go](../../internal/execution/worker_recovery.go)：已有 continue、原会话、新 Attempt 与未知操作守卫，不必另起恢复协议。
- [Codex lifecycle](../../internal/execution/adapters/codex/lifecycle.go)：SessionAvailable 当前依据合法 ID，尚非实际会话读取成功。
- [Orca lifecycle](../../internal/execution/adapters/orcatransport/lifecycle.go)：原终端可重接，替代终端恢复被拒绝。

## 2. 首轮范围

面向已建立原会话/工作区的执行，新增恢复行为以 implement 与 implement.report 为主。支持本地 Codex 原进程存活或已确认退出，以及本机/远端 Orca 原终端仍存在的观察恢复。publish/await_merge 沿用现行对账与交付门槛；不新增发布故障恢复选项。

用户可处理四类常见情况：中心服务/控制台重开；观察连接暂时断开；Codex 云端轮次明确失败；Codex 原 app-server 退出后继续同 thread。人工已在同 thread 续跑也归入最后两类，通过新的受管实施轮次重新核验当前工作区。

首轮不承诺完整主机断电恢复。Codex 原 home/thread 和工作区仍可核实、旧写者已停止时可以尝试同 thread 恢复；Orca 原 terminal/runtime 消失时仅显示不可重接原因。真实机器重启与磁盘耐久性未验收。

## 3. 最小恢复规则

沿用 Execution 的现有恢复状态与 adapter 事实；仅按需要补原轮次结局与原因，不把连接、会话、成果各自扩展成新的持久状态机。`completed` 仅代表原轮次结束，不能显示为任务成功。

服务自动进行只读核验：核对原身份 → 接回原观察或补读精确原结果 → 沿用原 Operation/工作区守卫 → 判断可否继续 → 更新原 Decision。核验顺序不能被 reconciling 提前返回截断。

| 事实 | 行为 |
| --- | --- |
| 原运行仍活动、身份一致 | 接回原 Attempt/turn 或 Orca 原 terminal/Command/状态基线；不启动新轮次 |
| 原受管轮次成功，完整结果及原 Command/turn 绑定可核实 | 幂等补记原结果一次，按现有流程推进；跨服务实例的后续输入仍需现有确认 |
| 原轮次明确 failed/interrupted，无未知操作，工作区可核实 | 保留失败及 final 缺口，退出无意义等待，进入 blocked/waiting_input 并提供继续入口 |
| 原成功结果缺失、输入效果未知、身份不符、旧写者不明 | 保持未核实，显示具体原因，仅允许再核验；不由超时推断失败或自动重发 |
| 原会话/Orca 原资源丢失 | 保留工作区与历史，显示原因；不自动创建替代会话/终端 |

已绑定的新旧结果仍使用现有 Command/turn/Attempt 校验、first-wins 事务和恢复版本。相同核验事实不反复生成 Decision 或递增版本；资格变化使旧答案过期。只读错误有界返回，不阻塞整个服务。

## 4. 一个继续入口，复用实施核验

复用正式 Decision/answer 与 `continue`，界面说明为“继续原会话并核验当前工作区”。`u` 仅查看，实际核验结果和 Decision 更新由服务写者处理；没有旧业务 Decision 的失败执行也必须有可用入口。

一次确认只消费为一个新的恢复输入 Command；后续步骤仍按现有流程与授权门槛推进。恢复输入按下面规则选择阶段：

- implement 中断/失败，或已经发送的 implement.report 失败，或原 source 失败/缺失：回到 implement，携带原任务契约、失败原因和当前工作区事实，核验已有修改并补齐必要工作，再生成新的 report source。
- 仅首次 report 尚未发送，且原 implement source 已成功：继续原 report。依据主干 PR #295，development/v3 已删除成果快照与候选 SHA；不恢复快照字段或比较，已发送报告的恢复统一重新实施核验。

102 的旧实施失败、人工新提交不匹配旧 source，按第一条处理。人工 turn 的成功 JSON 仅作提示，不填入旧 StageResult，不直接发布或标记交付。新实施可复用已有正确成果，也可在原任务范围内补齐；这是有授权的业务执行，不承诺只读接纳。未提交修改可以保留供实施处理，不为恢复另造内容摘要/候选接纳事务；后续 report/publish 沿主干 PR 流程验证原结果来源与实际合并交付。

原进程仍可用且轮次已停止时沿用原资源；Orca 还须原主 agent 状态已核实空闲、draft 为空且输入结果明确。仅观察重接不新建 Attempt。Codex 原受管进程树已确认停止时复用 beginWorkerRecoveryAttempt/BindRecovery，创建新 Attempt/generation/进程，保留同 Execution/worktree/thread。新输入前保存现有 Command/Operation 意图，未知回执只查原请求。

继续前重新检查原身份、恢复/Decision 版本、未知操作、会话可用性与资源资格。人工续跑的新轮次尚活动时不能因旧受管 turn 已失败而再启动输入；原会话空闲不可核实时保留未核实。原问题尚待回答时沿现有原生问答路径，不把该问题改造成重跑授权；首轮不增强问答投递协议。

## 5. 后端边界

Codex：复用公开 HistoryClient/原生读取接口核对冻结 home、thread、cwd 和精确受管 turn。原进程死亡时也能核实持久会话；读历史不发送业务输入，不将最新人工 turn 当旧 turn，不访问私有数据库/rollout。会话读取成功是可恢复的必要条件，实际 resume 失败须保留原因，不能退回 thread/start。旧写者未证实退出不启动替代进程。

Orca：仅重接原 runtime/terminal/incarnation/agent/worktree/branch 与已保存原生状态基线。依据主干 PR #296，原请求接受/开始、原 pane/provider 的可信 working→done 及完整普通回复仍是结果门槛；中心已结算保存的结果可脱离原终端重放，未保存且原输出不可读则不能补造。原生状态 gap、身份变化、回复不完整或冲突、目标替换明确诊断；不补 Enter、不替代 terminal。原终端丢失不等于旧写者停止。资源读取仅用于身份核验；不将 revision/dirty 恢复为阶段成果锁。

本机/远端共用上述 Orca 规则，首轮受控测试覆盖 environment 目标绑定；真实链先验本机。远端真实断连、新 terminal 原会话重建及主机重启留待后续范围确认。

## 6. 代码归口与解耦约束

恢复归属于现有 worker 执行域，由 Service 的单写者提交；不是把恢复状态放进内存 Worker 句柄。Worker/observer 可以在断连时不存在，持久 Execution/Attempt/Operation 必须仍能核验与恢复。两条路径分别是本地 Codex app-server 与 Orca 单终端；Orca 本机/environment 是同一 adapter 的目标差异。

| 责任 | 归口与现有代码 | 本轮整合方式 |
| --- | --- | --- |
| 服务换代后的恢复门槛、原操作未知、授权及新 Attempt | worker_recovery.go、worker_feedback.go | 保留唯一业务规则，继续执行前重验；不在两 adapter 各复制 Decision/事务 |
| 恢复事实、资格与版本 | worker_recovery_facts.go、worker_qualification.go、model/recovery.go | 归一化事实后统一分类、更新资格与恢复 Decision；不同入口不各维护一套判定 |
| 原观察与原操作补读 | worker_lifecycle_recovery.go、worker_lifecycle.go | 保留所需异步/轮询调度差异，消费同一事实与身份校验；共同规则归口，删掉被替代分支 |
| 完成证据、阶段结果与后继路由 | worker_completion.go、staged_flow.go | 共用既有结果结算入口；恢复只触发标准阶段命令，不另结算或发布 |
| 原生身份、历史、回执、资源资格及恢复绑定 | backend/LifecyclePort；adapters/codex、adapters/orcatransport | Codex 实现原 thread 读取/resume；Orca 实现原 terminal 重接及丢失拒绝，互不调用对方恢复实现 |
| 只读核验与展示 | recovery.go、recovery_inspection.go；app/product/console | 只组装事实、投影与提交正式 answer，不拥有恢复调度或直接写库 |

当前需要消除的具体结构问题：

1. resumeWorkerObservers/applyWorkerRecovery、resumeLifecycleWorkerFlow/resumeLifecycleObservation、recheckWorkerQualification 分别进入核验。允许不同 I/O 方式，但身份/未知操作/分类/Decision 更新必须复用同一组 worker 规则；不能增加第三条恢复入口。
2. worker_feedback.go 用 ManagedProcess 判断是否复用原资源，而 Orca VerifyManualRecovery 只给出 SessionAvailable；不能把“非本机受管进程”误解为“需要重建”。沿现有 LifecycleRecoveryEvidence/PolicyCapabilities 最小补齐通用资格：可沿用原资源、旧资源停止后可恢复同会话、不可核实/不支持。它们是动作资格，不是新的持久状态机；原生取证由 adapter 提供，不能让 Orca 伪造 ManagedProcess=true。
3. u 的读取路径不能依赖运行态 owned process/resource 回调必然存在。复用冻结 snapshot/setup 和只读端口装配，缺少能力明确未核实；不调用会登记 observer、维护运行态资源或提交状态的恢复方法。允许共享只读装配函数，不增加第二个 RecoveryService。

业务事实由 worker 单写者接受并保存；adapter 不持有中心存储、Decision/StageResult 写权限，也不选择 implement/report/publish 后继。共享 worker 不导入恢复 adapter、解码原生 envelope 或按 backend/protocol 名称分支；必要能力差异通过现有 lifecycle 契约表达。只读报表与服务分类共用可用的事实语义，报表不复制一套授权规则，也不能代替提交时重验。

实施时在既有文件内整合触及的规则，删除重复/不可达路径；不默认建新包、RecoveryManager、策略注册表、恢复事件总线或全部 worker 提取工程。接口字段只在既有契约确实不能表达两条路径资格时新增，并同步 adapter 与非平台 fixture。交付需验证共享规则能消费 fixture 事实、不依赖进程 kind 或平台名称；结构验收 CR-A07 复用已有边界测试，不增加 L3 顶层场景。

## 7. 最小交付与后续边界

首轮为一次普通开发：现有 worker 恢复归口、共享分类/Decision 修正、Codex 原 thread 核实和恢复、Orca 原终端重接/丢失诊断、现有继续入口与实施路由、最小 L3 驱动。六项功能验收 CR-A01–CR-A06 加一项结构验收 CR-A07，三组 controlled（为保持每入口 5 分钟上限拆成独立场景）与两条 live，详见 Plan。

原十二项 DR-A 与九条场景属于被替代的全面草案；本次验收以 CR-A 项为准。后续事项只在范围表保留，不作为首轮前置，不编写占位测试。保留未知发送/发布不重试、原 final 来源、候选隔离及合并后唯一 Delivery 等既有约束；减少增强范围不放宽这些条件。

新专用“接纳成果”产品流程无已证实必要性，不预定第二期必做。只有实际使用证明实施重核成本不可接受时再评估。硬件断电验收不占本轮自动项，也不阻塞无关功能；产品声明对应保证前须另行验收。

## 实施补充

恢复资格读取沿用 worker_qualification 的异步探测和单写者接受，包含无内存 observer 的执行；原目标/Attempt/generation/Command/恢复版本变化使迟到探测失效。原生结果可信时，观察连接错误不得写入该结果的业务 Failure。Codex 每个受管 Command 在模型输入前保存原 home/thread/workspace 的公开历史绑定，而不只为首轮保存。

受控夹具实测单个项目约需 3 分钟，原多现场顶层测试超过 5 分钟。按 Plan 的既定拆分规则保留三组覆盖，拆成独立测试入口，每入口时限仍为 5 分钟；不增加产品能力或全排列故障。入口与执行证据见 L3 文档和专属验收。主干已删除 workerreturn 帧契约；本轮沿用普通回复和原生状态，不恢复旧包或其依赖白名单。

## 2026-10-13 主干整合与审计修正

基线更新至 `efd492ca`（PR #296），同时保留 PR #295 的 development/v3 与实际 PR 合并交付。Orca 恢复资格只读查询原 pane/provider 的主 agent；屏幕 idle 不再授权继续。发送回执丢失且没有原请求接受/开始证据时，普通回复和 done 不能补造原接收事实，保持未核实及零重发。原已保存结果不依赖终端存活。异步资格覆盖 current/pending Command/turn，恢复新 Attempt 在 native BindRecovery 验证通过后才修改 actor 记录；Codex PID 身份不明不视为原资源可用。

2026-10-13 后续基线同步 `da325443`（PR #297），保留本机配对、就绪等待及 explicit rejected 回执。已准备 session 或明确被拒绝的输入不是未知发送：worker 共用原 Operation 事实，在正式确认前保持门槛，确认后分配新 Command，绝不重放原 rejected 请求；unknown/accepted 不能借此变成 unsent。此判定同时用于原观察入口、资格和报告反馈，不在两个 adapter 复制业务路由。

结果回执只证明原轮次，不更新当前会话可用性；会话资格继续由 worker 的核验事实更新，缺省 false 不造成恢复版本变化。Orca 原生状态缺口使用现有 InputWait 诊断投影，保持原输入、无结果与零重发；若原主 agent 仍 working，不能为了展示“未核实”伪造轮次已结束或新增可继续选项。
