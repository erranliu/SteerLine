# Spec 03：最小状态机的分阶段远程闭环与 viewer 人工决策

状态：待实施。日期：2026-09-23。

前置：[Spec 01](01-orca-remote-protocol.md) 必需能力通过真实远端验证；[Spec 02](02-execution-model-and-adapter.md) 模型、存储、迁移和执行接口完成。依据：[执行架构共同约束](execution-architecture.spec.md)。

## 1. 目标与范围

中心 SteerLine 领取一个业务任务，通过 Orca 在远端独立 worktree 中逐步执行 implement、verify、review、publish_pr，viewer 显示阶段、输出和人工问题；用户回答后同一权威步骤尝试继续，经中心核实各门槛、PR 合并及结项后任务完成。中心重启或网络暂断不会重复派发。

生产调度保持一个活动 Execution，运行 Spec 02 的 `minimal-v1`；每个步骤可以使用项目基础技能或脚本，由中心控制顺序和门槛。本阶段不实现任意流程/DAG 编辑器、多 agent 并行、移动端、多租户或公开 HTTP 服务。数据与命令接口必须沿用 Spec 02，便于后续接入这些能力。

## 2. 配置与接入前置

新增显式的受监管远程配置，至少包含固定 connection ID、home Orca runtime/协调者绑定、environment ID、远端 repo selector、agent、输出/状态轮询参数和能力报告版本。具体配置字段由实施确定并更新示例；旧本机 Orca 配置继续采用原模式。

配置目标是远端时，fallback 固定为 fail。任何创建前后的错误都不得改在中心执行。能力缺失、连接身份失效、仓库不可访问等问题在任务领取前尽可能发现；领取后发生的问题必须保留任务与执行记录。

远端仓库包含已提交的项目步骤入口和必要脚本，project_steps 满足 Spec 02 契约。派发载荷使用远端相对路径、版本化引用或完整内容；不能引用中心 prompt/result 文件、中心私有 Git ref 或 Beads 数据库路径。旧的整段 start/finish 入口若不能在指定步骤停下，必须先拆分接入，不能直接用于 minimal-v1。

任务来源与结项的权威留在中心。远端技能获得任务内容与已领取事实，不调用 `bd claim/close`，不复制数据库。若既有 start/finish 技能强制要求本机 Beads 访问，必须先提供并验证远程接入契约；不能只在提示词里宣布该依赖不存在。

初版结项支持中心能执行的原生 Beads close，以及明确声明 `execution_location=controller`、不依赖远端 worktree 的 completion 适配器。旧 `{worktree_path}` completion 和必须访问远端现场的适配器在远程配置预检时拒绝。其后续支持需 Spec 01 的脚本能力通过验证，不能把未支持的配置带入等待合并后才失败。

## 3. workspace 与派发

1. 中心完成配置预检，固定目标仓库/分支、技能协议版本及 base commit，持久化 WorkflowRun、Execution、StepAttempt 和任务领取意图。
2. 由中心领取任务并回读核实身份与领取者，之后才允许派发 agent。claim 响应不确定时保持待核实并回读，不能直接再次 claim 或派发；被他人领取时记录冲突并结束本次准备，不改写对方领取。
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
| 明确停止操作得到平台停止证据 | Attempt/Execution=stopped；记录谁请求以及停止结果 |

所有非法转换、身份不匹配和旧尝试消息保留诊断证据，不推进当前流程。输出文字、TUI idle、进程退出码或心跳单独都不能替代业务结果。

没有输出超过 stall_timeout 只产生需关注提示；有可能仍存活时，不因此自动杀进程或重派。agentWait 对象显示“宿主交互待处理”；null 表示本次检查没有等待证据，缺失显示未知。只有真实 Orca ask 才进入业务回答入口。

## 5. 消息、输出与人工回答

中心独占本项目配置的协调 inbox 消费职责；viewer 不直接消费或 ack Orca 消息。使用有界 check 等待并行执行状态/输出轮询。默认状态与输出目标刷新间隔为 2 秒，单次网络操作有超时，不得因长等待阻塞命令提交或 viewer 查询。

按 Spec 01 的实际 Ack 语义持久化整个 Delivery，并处理所有消息。未回答问题需要保留原 Delivery 时，仍用已验证的观察/只读消息读取更新界面；新消息和完成事件不能因用户暂未回答而永久不可见。完整入库与业务应用分别记录，重放不重复追加规范事件。

worker-read 优先可用的 transcript，回退时明确标记 terminal。source/cursor 一起持久化；只有内容写入成功后推进 cursor。截断、历史淘汰和 source_changed 写入显式 gap 事件，不以“无输出”掩盖。展示输出不参与决定任务完成。

远端接入复用 [控制台 4A](pr-04a-execution-backends.spec.md) 与 [4B](pr-04b-conversation-events.spec.md) 的执行/会话契约及同一 UI，不新建远端专属消息模型。分别验证远端 agent/协议版本的 turn、phase、final、历史与等待能力；本机证据不能代验。精确折叠遵循 [4C](pr-04-console.spec.md)，缺能力采用明确的执行级展示并保留未验收项，不能因 worker_done 或 terminal 静默而制造 final。完整正文、晚到回复、source segment、去重和游标恢复沿用同一存储契约。

回答走 Spec 02 的 SubmitAnswer 命令：精确 decision ID、attempt、expected_version、选择/备注及幂等键。服务事务检查问题仍有效、答案符合请求、目标尝试仍权威，然后保存答案和 reply Operation。重复同一提交返回原结果；不同答案竞争时返回冲突并展示已接受答案。

离线时允许保存有效答案，viewer 显示“已保存，待远端送达”。同一业务问题的回答不因重连再次生成新消息。若旧问题或 Dispatch 已失效，答案记录保留并标记 failed/superseded，不发送给替代 worker。

业务自由文本答案与 Orca 终端按键输入分开。登录、信任目录和宿主权限提示展示明确处理原因；本阶段不通过 viewer 自动回答这些提示，也不把其处理后果伪装成业务决策已应用。

## 6. 结果、PR 与结项

按 Spec 01 接收与受接受 worker_done 对应的结构化结果，核验业务身份、平台身份、工作区事实和结果版本。缺失、冲突或超限结果进入 blocked/result_invalid，即使 Orca 已结算成功也不能宣告业务成功。

每个阶段结果按 step_id 校验各自 schema 和 input/output revision。实现提交的候选 commit、验证回执、审查结论及发布结果之间保存明确关联。通过门槛是中心的持久状态，不接受 agent 在最终总结中一次性自报前面阶段全部完成。

publish_pr 成功结果包含精确 PR URL、实现 head commit、目标分支与固定 base。中心向平台验证实例/仓库、PR 源与目标、结果 commit 对应关系及实际 merge 证据；不得按相似标题寻找 PR。必要的 Git commit 核验在中心仓库或平台可验证接口完成，不读取远端绝对路径。发布后的 PR head 变化需重新核验并记录，不能把旧 commit 上的测试/审计证据自动算到新 commit 上。

发布请求响应丢失时，查询精确仓库、源分支、目标分支及记录的发布身份，核实原 PR；不能使用模糊标题搜索，也不能直接新建第二个 PR。无法唯一确认时保持 outcome_unknown 等待处理。

等待 PR 合并时释放执行槽，保留任务领取、选用结果和 PR 绑定。worker 资源按 Orca 规则完成 release/明确的保留处理；释放 agent 终端不等于删除 worktree，远端产物默认保留，不触发公开发布。

合并后在中心执行已固定的结项适配，并回读任务来源确认 closed 后才 completed。结项的外部副作用采用同一业务幂等键，部分失败继续 completing 并对账，不重复开发、不绕过自定义结项回调。PR 未合并关闭或绑定漂移则 blocked，不能关闭任务。

停止整个服务仅停止中心监督并保存状态；远端 worker 可以继续。取消某次执行是独立、持久化且可追踪的 Stop 操作。启动恢复先处理已有消息、动作和执行身份，再考虑派发新任务。

## 7. viewer 与命令行

现有 TUI 通过 QueryService 和 CommandService 读取/操作新记录，旧快照仍可只读查看。CLI watch/answer 使用相同入口；选择 task ID 存在多个目标时必须要求 execution/decision ID，不能默认选第一项。

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
| R07 | 任一步骤完成期间中心停止 | 重启读取原结果并恢复正确的下一步骤；仅发布结果进入 PR 核验，不从头开发 |
| R08 | 网络中断、remote runtime 重启、身份替换 | 显示 unverifiable/身份变化；不操作替代终端，不把未验证退出算完成 |
| R09 | worker 退出却无结果；结果身份错误；重复 worker_done | blocked 或去重，不错误关闭任务 |
| R10 | 输出分页/source_changed/历史淘汰 | 已保留内容可读，缺失被标记，不影响持久业务消息 |
| R11 | task 的另一 Execution/旧 attempt 消息到达 | 仅进入对应历史；不改变当前 agent 的状态或回答目标 |
| R12 | PR 实际合并、PR 未合并关闭、结项部分成功 | 分别完成/阻塞/继续对账；task close 不重复产生副作用 |
| R13 | local 路径或远端 completion 不支持 | 在预检拒绝或明确阻塞，不以本机同名路径替代 |
| R14 | 服务停机与显式 Stop | 前者不取消远端；后者确认目标停止后才释放相应许可 |
| R15 | claim 成功但回执丢失、领取者发生变化 | 中心回读核实后才能派发；不存在双重领取或对错误所有者启动执行 |
| R16 | 验证失败、审查请求改动、修复预算耗尽 | 前两者由中心安排有限修复并重新检查，耗尽后等待人工处理，期间不发布 PR |
| R17 | 验证通过后代码改变或旧 verify 结果迟到 | 发布门槛不通过；正确关联当前 revision/attempt，重跑已失效检查 |
| R18 | 同 worktree 从 implement 切到 verify/review/publish_pr | 每步有独立身份，无重叠修改；worktree 保留，故障恢复不串到旧步骤 |
| R19 | 特殊 PR 技能越步修改代码、提前返回 PR 或发布回执丢失 | 不越过门槛；标记越界/不确定并核实，未确认前不再次创建 PR |

协议替身和存储故障测试覆盖所有分支；真机至少执行 R01、R02、R06、R07、R08、R16、R18，并在测试托管仓库完成 PR 合并/结项路径。真机结果、版本及残留资源记录归档；无法实施的场景保持未验收，不能用本机替身替代。

## 9. 完成定义

远程配置有可复现样例；上述必需验收通过；旧 local/本机 Orca 行为回归通过；hosting、state-model、activity、decisions、skill-contract、completion 和 data-contract 文档按实际实现更新。

完成后中心状态机实际控制准备、实现、验证、审查、发布、等待合并和结项；用户能在现有 viewer 看见各阶段、回答业务问题并恢复监督。项目基础技能/脚本只完成被派发步骤。并行 agent 和移动端尚未上线，但无需改变身份归属或消息寻址即可继续实现。
