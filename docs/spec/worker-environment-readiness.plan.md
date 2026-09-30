# Plan：worker 环境就绪与宿主阻断恢复

目标：`WORKER-ENVIRONMENT-READINESS`。唯一执行依据：[Spec](worker-environment-readiness.spec.md)。状态：已实施并通过最小验收，PR160 待审阅。日期：2026-09-30。未分配原生 Task ID；基于当前 worker-flow 和本机 Orca 原生实现，一个实现 PR 交付。

## 1. 核实接口并收敛证据

围绕 Spec §3–§5 核对安装版本 Codex 的权限配置、Orca 原生轮次、terminal agentWait/退出、worker-read 输出及请求恢复能力。当前固定 model 启动命令已有 model/effort/sandbox，但未显式设置 approval policy；model 为空时还有另一条 agent 启动分支，两者必须一起收口。原生 poll 已处理 inbox 和 relay，尚未将 agentWait 贯通到主阶段待决策，且在无 Dispatch/上轮结算等情况提前返回。先核对这些实际入口，不另建后端适配框架。

只补证一个关键边界：同一 Orca agent 会话的命令工具探针及准备轮次结算，能否提供 Spec 要求的执行证据。现有 `WorkerOutput.Transcript` 是原始 JSON，`Terminal.Tail` 是屏幕文本，均不是现成的类型化工具成功事件。核对实际版本的 transcript schema 和来源可信度，识别工具调用/结果并绑定探针摘要及 nonce；在平台适配层提供事实，不要求 Orca 提供 Codex UUID。成功文件、assistant 正文和宿主命令均不能单独作为执行证明。

在这一步先完成最小原会话探针实验；若平台不能给出所需证据，记录具体缺失并将实施标记为阻塞，不开展一套只会拒绝执行的“完整实现”，也不宣称交付。本文定义的是实现目标，不宣称相关平台接口已经全部验证。

| 代码入口 | 本次职责 |
| --- | --- |
| `internal/steerline/config.go`、`execution_profiles.go`、`service_execution_profiles.go` | 显式 Windows sandbox、冻结权限、静态能力检查及配置诊断 |
| `internal/steerline/orcaworker/command.go`、`worker_flow_orca_native_model.go`、`worker_flow_orca_native.go` 的准备入口 | 有/无显式 model 统一受管启动，固定非交互权限、校验 args 冲突、保存原会话有效配置 |
| `worker_flow_orca_native.go`、`worker_flow_orca_relay.go` | readiness 与 business 轮次区分、独立观察、原生结算与恢复对账 |
| `internal/steerline/orcaworker/terminal.go`、`output.go`、`conversation_service.go` | 原 terminal 等待/退出事实、工具证据及来源，复用现有会话事件 |
| `domain.go`、`worker_flow_state.go`、`model.go`、现有 CommandService/Operation/store | 原生轮次类型、准备事实、blocker、反馈中的中心动作及权威事务提交 |
| 现有执行/Decision DTO 和控制台 | 冻结权限、阻断可信程度、原 terminal 与重新检查入口 |

完成条件：实际命令/回执契约及最小工具证据已补证；新增字段所有者和写入事务明确；ER1–ER4 的证据来源已对应到真实入口。不复制另一套 TaskStore 或主状态。承接实施时主干已有的类型化平台事实和原子提交入口；与[协议边界](architecture-01-worker-protocol.spec.md)、[原子提交](architecture-02-worker-commit.spec.md)的收口方向一致，不另写 Service 原始协议解析或隐式全量写回路径，也不把这两个未完成 PR 任意设为本任务的硬前置。

## 2. 冻结权限并持久化准备事实

实现 Spec §3 的 Profile 字段和参数冲突检查；在领取前只做无启动副作用的配置、CLI 能力与 Orca 连通性检查，并展示配置不可用原因。领取事务核对所选 Profile digest，不在失败后改选配置。Windows Orca Codex 有/无显式 model 均通过受管命令设置 `approval_policy=never`、`workspace-write` 和选择的 `windows.sandbox`；model/effort 未配置时保留启动器默认解析语义。领取时保存固定配置，资源实际绑定时保存 runtime、写入根目录和启动命令证据，不预填未知资源。

在现有 `domain.go` 原生轮次记录区分 `readiness` 与 `business`，保存准备 nonce、输入摘要、开始/截止时间、收据校验、工具证据和结算结果。明确当前原生准备/业务轮与业务轮计数、业务反馈目标的关联：准备轮次不能成为业务的 CurrentTurn/LastStoppedTurn 或消费业务反馈，原会话仍最多一个活动/未知原生 Dispatch。不同准备意图使用新 nonce，同一意图重启保留原值。阻断字段保存 reason/source/confidence/observed identity/time，关联已有 Decision 和恢复 Operation；原 question/message、已保存答案及投递状态分别保留。只增加实现所需的字段，不提前设计跨 agent 健康服务或成功缓存。

首次准备预算从领取后准备意图开始，资源创建、识别、注入、探针和结算共用冻结的 `orca.wait_timeout`；业务首次启动另保存原派发意图的启动观察期限。`command_timeout` 约束单次 I/O；主循环按有界查询持续对账，不使用长时间内循环耗尽整份预算，不因服务重启或步骤变化重置期限。

按项目规则更新当前持久格式校验；不迁移已运行的旧 Execution 或兼容缺少必需配置的文件。在外部副作用前拒绝不支持的记录，当前格式的中心重启恢复须同步实现。

完成条件：有/无 model 两条配置均落实相同权限；包括快捷绕过、config/Profile 和目录覆盖在内的 args 冲突被拒绝；固定配置与资源绑定分开核验；格式与身份验证覆盖新增字段。对应 ER1、ER2、ER4。

## 3. 在原会话接通就绪准备

在首个业务 Dispatch 前创建一次 readiness 原生 Task/Dispatch，复用原 worktree、agent terminal、coordinator 和 Orca Run。准备输入、探针目录及 exclude 由中心在自身权限下准备；探针实际执行和成功收据由原 agent 的命令工具完成。输入、临时文件、收据和 relay 精确路径分离，校验最终路径及 reparse point，清理仅删除本轮自建文件。

将准备事件分流到准备事实：不调用业务 begin、PR 绑定或业务完成处理；准备问题仍通过原 message ID 持久化/回答，不解释成项目实现要求。中心验证身份、nonce、摘要、工具成功结果、原生成功结算及当前无 blocker，全部满足才保存业务派发意图。记录与消费标记先于 Delivery ack/文件清理。准备成功写入与后续业务派发之间沿用既有 Operation/事务边界，重启后能判断“还未派发”和“原派发待对账”。

明确探针失败、收据缺失、失败结算和结算未知的区别；准备绑定前的错误也形成统一待决策。截止后晚到成功只补历史，显式重新检查并核实当前目标后才可接纳原轮成功；新准备必须在原轮已结算后创建，并拥有独立预算。原请求未知时只对账，禁止重复注入。清理准备文件不删除业务 relay，不依靠清理成功判断轮次完成。

完成条件：ER1 成功/失败分支均通过；在真实同一会话证明工具读写删除、准备成功结算与一次业务派发，记录权限及身份。缺少真实平台执行证据时此步骤未完成，不能仅以 fake 回归交付。

## 4. 独立观察与统一待决策

调整准备入口与 native poll 的观察顺序和错误汇聚，使原 inbox/relay 不可用、无 Dispatch 或上轮已结算时仍可获取已绑定原 terminal 的等待、退出和有界输出。保留现有正常 worker_done 路径；宿主诊断不合成业务结果，不遮蔽结算和 PR provider 观察。

由平台适配层按 Spec §5 形成 typed 宿主观察，区分确认工具错误、结构化等待、prompt-text 疑似等待及观察缺口。采用原身份和来源事实去重，过滤旧轮次与旧 incarnation；无法归属轮次的 terminal tail 不驱动当前业务失败，一般命令非零退出及单次输出截断不触发环境故障。无法归属轮次的当前 agentWait 作为会话级疑似等待阻止新输入，保持真实性限制。

通过现有中心写者在核对 WorkflowRun/turn/Decision 版本的事务内提交 blocker、会话事件及必要主阶段变更，保持原 Dispatch 的活动/未知事实。不把仅等待 PR 合并的 terminal 故障改成业务待决策，不让迟到宿主提示重开已交付任务。业务启动观察到期才把缺少开始事实转待核实，正常业务执行不新增总执行时限。

更新读取 DTO 与最小控制台显示：原因、证据来源/可信程度、冻结权限、原轮次状态以及可用恢复动作。保留原 question 及答案投递状态；宿主等待期间给 worker 的输入只保存、不注入菜单，原轮结算后问题按既有规则过期，不盲目恢复待回答。

完成条件：worker 无法写 relay 仍可见阻断；ER2 可信度、去重、业务问题并存及错误分类通过。敏感 capability 不进入数据库、日志或 UI。

## 5. 实现 typed 恢复与重启对账

在既有反馈命令内接入“打开原 terminal”和“重新检查环境”中心动作，明确选项受理方和固定目标/版本，复用 CommandService 与 Operation 幂等边界；不新增专用恢复命令族，也不通过自由文本关键词触发重派。保留完整备注及给 worker 的待投递文字。尚未绑定 terminal 时只允许原准备对账，不提供无法定位的打开动作。

重新检查先读取原 runtime/terminal/incarnation、agentWait、原 Dispatch 及未完成请求；未知回执先对账，不绕过原轮生命周期发送探针或业务任务。确认环境故障使旧就绪事实失效，须原轮结束后在同会话重验；只有观察断线而无确认故障时恢复原轮观察即可。超时后的完整原准备结果由显式重新检查核验后接纳，不自动推进。

动作受理及结果提交都核对绑定的准备意图/轮次和 Decision 版本，同一 blocker 只有一个未完成重新检查 Operation；即使连续点击使用不同 command ID，也关联原未完成意图而不再次操作。旧动作不能影响已经开始的新轮。打开 terminal 只定位界面，不记录为批准/修复完成。

分别实现未派发业务、业务仍活动、原轮已结算、原会话不可核实的恢复分支。只在同一原会话及轮次条件满足时重新做准备或继续业务；不改变配置、不补建替代 terminal、不重复领取、不 reset worktree。readiness 不改变既有业务 feedback 的目标/命令身份；过期目标保留并提示，不自动改投。解除 blocker 后只恢复仍活跃、仍待回答的原 question；投递未知的已保存答案先对账。

服务启动恢复准备期限、证据、blocker 和原操作；重复动作、重启、迟到收据采用同一身份规则。对原准备轮不能结算的场景给出保留现场的明确结果，不能把“菜单消失”当准备成功，也不能无限自动重试。

完成条件：ER3、ER4 通过，尤其验证响应丢失后零重复派发及 deadline 不因重启延长。

## 6. 最小验证与文档交付

优先复用 `TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal`、`TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement`、`TestNativeOrcaRejectsLateDispatchMessages` 及 Orca fake，按 ER1–ER4 补充受控回执/故障场景。新增字段/来源/收据校验用同一 fixture 或表驱动断言，不为每项建立一套真机实验；不另做全面 Windows 故障矩阵，不破坏宿主沙箱来造失败。运行受影响的中心用例和 `orcaworker` 包，并使用仓库快速入口：

```text
go run ./cmd/test-feedback -quick
```

`-quick` 当前固定只覆盖少量中心闭环及纯规则包，不包含全部原生 Orca/工具证据用例，不能用其通过替代上述定向验证；可用快速入口追加 focused package/test 参数并带 `-count=1`。只因失败或具体未覆盖风险扩大测试；确需全量时使用 `go run ./cmd/test-feedback -workers=3` 并说明原因。配置/共享 DTO 触及 local 时只补对应回归，不扩展多平台验收。

在隔离测试项目完成一个真实准备→业务派发闭环，记录构建版本、冻结 Profile、准备/业务 turn、原生 Task/Dispatch 和原 terminal/incarnation；其余阻断/重启用受控故障证据覆盖。报告真机与 fake 的边界，不把未测试的 OS 根因修复写成支持能力。

更新 [Spec 索引](README.md)、`docs/data-contract.md`、`docs/decisions.md`、相关运行配置示例及原生 Orca 实际支持说明。明确 Windows sandbox 必需配置、宿主操作入口、未知轮次不能盲目重派，以及不自动恢复已丢失会话。

完成条件：ER1–ER4、一次真实闭环、相关快速回归及文档一致性可审阅；平台工具证据缺失或成功链路未验证时不能标记实施完成。纯 Spec/Plan 编写仅检查链接、格式和语义；本 Plan 不授权生产部署、生产数据库更改、现存任务 101 迁移或代用户合并 PR。
