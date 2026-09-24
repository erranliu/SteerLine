# Spec 01：Orca 远程能力验证与执行协议

状态：待实施；本文不代表已完成真实远端验证。日期：2026-09-23。

依据：[架构决策](../architecture-direction.md)。后续：[Spec 02](02-execution-model-and-adapter.md)、[Spec 03](03-remote-execution-and-viewer.md)。

## 1. 目标与交付

证明中心 SteerLine 可以仅通过 Orca 完成远程 agent 派发、观察、提问/回答及结果接收，并确定可供后续实现使用的版本化协议。远端不运行 SteerLine 服务，不依赖中心与远端共享磁盘。

交付包括可重复运行的探测工具、脱敏协议样本、能力报告和对本文所述逻辑协议的 Orca 字段映射。报告必须记录中心 CLI/runtime、远端 runtime、agent、操作系统和测试时间。文档声明、CLI help、协议替身和真机验证分别标注，不能相互替代。

本阶段不实现业务调度器、移动端、多用户共享服务、多 agent 并行或完整脚本状态机。测试使用独立测试仓库和任务，不领取生产 Beads 任务，不操作生产 PR。

## 2. 前置条件与完成门槛

- 中心已有可调用的 Orca CLI/runtime；远端已有配对的 Orca runtime、agent、注册仓库和必要仓库工具。
- 使用显式 environment ID、远端 repo selector 和 agent 配置。凭据由连接配置提供，不能写入任务提示、协议样本或结果。
- 没有真实远端时，只能交付探测工具及协议草案，能力报告标记 `not_tested`；不能把 Spec 标为验收通过。
- 能力结论采用 `verified / unsupported / not_tested`，并记录证据。必需能力存在后两种结果时，Spec 03 的对应生产路径不得启用。
- 能力按中心与远端的版本组合判断；升级后至少重跑派发、决策、结果和恢复兼容性探测。未识别字段保留或忽略，不能当作已支持某项能力。

## 3. 职责和寻址

SteerLine 持有业务任务、执行身份、决策和完成规则；Orca 持有受监管执行及跨机传输事实。每个探测使用独立 Run 和精确 Dispatch，不通过终端标题或任务正文推断身份。

采用已验证的 `worker-start --on <environment>` 选择远端。创建远程 worktree 时指定 `new-top-level` 与远端精确仓库；不使用远程 `current` 或由中心 CWD 推断仓库。

派发后通过 Run 所在 Orca runtime 按 Dispatch ID 调用 worker 操作和消息操作。不能把 `--on` 机械地加到所有后续命令，也不能把远端 terminal handle 当作中心 handle。终端可能不存在；正常监督必须能使用 worker 接口。

必须验证中心服务在普通后台进程中如何建立、持久保存、恢复协调者身份。若需要 Orca 管理的协调终端，应使用明确的配置和身份校验；不得依赖当前活动窗口。中心 SteerLine 重启、Orca 重启及终端身份失效需分别验证，不能承诺失效身份可自动重新绑定。

## 4. 能力矩阵

| 能力 | 需要验证的内容 | 门槛 |
| --- | --- | --- |
| 后台协调者 | 非交互调用、Run 绑定、重启后的读取/回答权限、失效身份诊断 | 必需 |
| 远程派发 | worker-start 返回身份、ready/failed/outcome_unknown、残留资源、精确 host/repo | 必需 |
| 同工作区分阶段执行 | 前一步结算后在精确远端 worktree 派发下一步；新 Task/Dispatch 身份、原分支/代码保留、无两个步骤同时修改 | 必需 |
| workspace 准备 | 远端可获取固定 base commit；实际 branch/HEAD 可核验；提示可在远端读取 | 必需 |
| 状态观察 | worker-show/list 的 live/unverifiable/exited、agentWait 的对象/null/缺失语义 | 必需 |
| 增量输出 | worker-read 的 cursor、分页、source_changed、保留边界和已释放后的可读性 | 必需；历史缺失可显式降级 |
| 结构化消息 | 跨机 status、question、worker_done 的身份、顺序、重放、大小限制 | 必需 |
| 人工决策 | ask 超时后同 ID 恢复；reply 的接收语义、重复回答及响应丢失恢复 | 必需 |
| 完成报告 | worker_done 的接受条件、payload 保真、失败结果、资源结算 | 必需 |
| mutation 恢复 | request ID 何时可取得、retry-request/request-show 适用范围、无回执时的边界 | 必需验证；自动恢复可降级为人工核实 |
| 停止和释放 | stop 与 release 的区别、断线时的 unknown、仅操作目标执行 | 必需 |
| 步骤脚本回执 | 受监管步骤调用仓库脚本，回传命令/脚本版本、退出码、检查结果和对应 commit；失败不能变成自然语言成功 | 必需 |
| 无 agent 的脚本步骤 | Orca 启动确定性脚本、输入传递、退出/结构化回执、重启后取证 | 探测必做；能力不足不阻塞首版 agent 步骤 |

无 agent 的直接脚本探测若不通过，报告必须写明缺口；不能把“让 agent 调用脚本”当成已验证的直接脚本执行能力。首版可用受监管 agent 包装某个脚本步骤，但必须验证脚本自身的结构化回执。中心分别派发 implement、verify、review、publish_pr；不能用一次 `execute_workflow` 让 agent 自己推进这些步骤。

## 5. SteerLine 逻辑协议 v1

以下字段是 SteerLine 协议，不是对 Orca 原生 wire 字段的假定。适配器映射必须通过真机样本确认；若 Orca 自定义 payload 无法保真，标记不支持并修订协议，禁止退回解析自然语言终端摘要。

### 5.1 身份与请求

每个请求绑定 `project_id、task_id、workflow_run_id、execution_id、step_id、attempt_id、operation_id`。中心生成这些 ID，并在外部调用前持久化。Orca 返回的 home runtime/Run/Task/Dispatch、execution host 和 worktree 身份另行记录，不与业务 ID 混用。

`operation_id` 是中心去重键；只有经验证的 Orca 请求映射才具有远端重放语义。不得假设将该值放进 prompt 就能防止重复启动。

派发内容必须自包含：任务/Spec 内容或远端可访问的版本化引用、目标仓库、目标分支、固定 base、技能相对路径及协议要求。中心 `prompt.txt`、本地 Git ref 和 `BEADS_DIR` 不得直接作为远端可用资源。agent 不能自行领取或关闭业务任务。

每次只派发一个步骤，携带 workflow_version、当前步骤契约、允许操作、input_revision 和必需前置结果 ID。后续步骤必须由中心在校验前一步后新建尝试；worker 不能根据任务正文自行启动下一步骤。首版不要求维持同一个 agent 会话，只要求保留同一 Execution 的工作区和完整阶段上下文。

### 5.2 事件信封

归一化事件至少包括 `protocol_version=1`、上述业务身份、`external_message_id` 或已验证的观察来源、Orca Dispatch 身份、`kind`、`received_at` 和 `payload`。远端时间可附加，但不作为跨机排序依据。

| kind | 语义 |
| --- | --- |
| observation | 主机提供的存活/等待事实，带 observed_at 和来源 |
| output | transcript 或 terminal 输出，携带来源和不透明 cursor |
| progress | agent 主动报告的阶段或心跳；不证明完成 |
| question | Orca ask 的持久问题 ID、问题正文及允许的答案 |
| decision_applied | worker 确认已读取某个答案并开始据此继续 |
| result | 与受接受的 worker_done 对应的结构化步骤结果 |

观察事件与业务完成消息分开。一个活动终端不等于 agent 正在推进；keepalive 仅表示等待命令仍活着。

### 5.3 问题与答案

问题通过 Orca ask 发出；超时不创建新问题，应恢复原 message ID。Spec 01 必须锁定 Orca question 字段到 SteerLine question/options 的实际映射。首版至少支持正文、自由文本答案和能无损表示的选项；上下文、推荐项等可选字段仅在能力已验证时启用。

答案绑定原问题、Dispatch 和 attempt。Orca reply 成功只记为平台已接受，不自动记为 agent 已应用。派发协议要求 worker 在 ask 返回后发送 `decision_applied`，或在最终结果中携带 `applied_decision_ids`；只有相应证据才能确认应用。

Orca 自身的登录、信任目录和权限提示可能只有 agentWait 观察。不能把它们伪造成业务 ask，也不能从提示文字生成未经验证的自动按键回答。

### 5.4 结果

结构化结果必须包含：全部业务身份、`outcome=succeeded|failed|blocked`、摘要、错误及版本。与 worker_done 的 `succeeded|failed` 对齐规则：业务 blocked 使用失败 settlement，并保留 blocked 原因；等待人工回答使用 ask，不能提前结算 worker_done。

所有步骤结果包含 input_revision、output_revision、实际 worktree 身份/分支和已应用的 decision IDs。workspace 身份与 host 观察一致；报告只属于当前 step_id，不能以其内容要求中心跳过后续步骤。

| step_id | 额外必需结果 |
| --- | --- |
| implement | 可核验的候选 commit、工作区代码无未提交变更的检查、实现摘要；本地提交用于固定验证对象，不包含 PR 发布 |
| verify | 对同一候选 commit 执行的必需检查清单、脚本/命令摘要、各退出码、passed/failed、证据引用 |
| review | 对同一候选 commit 的 passed/changes_requested、问题清单、审查契约版本；审查只报告，不在此步骤修复代码 |
| publish_pr | delivery=pr_submitted、精确 PR URL、仓库/目标分支、base commit、已发布 head commit、所依赖验证/审查结果 ID |

脚本执行失败必须报告 failed 并保留退出码；审查正确完成但发现问题，可以 outcome=succeeded 且 verdict=changes_requested，中心不得据此通过审查门槛。只有 publish_pr 的结果会触发平台 PR 核验；其他步骤返回 PR URL 不能推进至等待合并。

报告路径和产物引用均可留在远端，携带 host/worktree 身份。它们不能被中心作为本地路径打开，也不能在缺少关键字段时替代结构化结果。

必须确认这些元数据能与同一次受接受的 worker_done 一起保真回传。两段消息拼接仅在有明确 result ID、Dispatch 绑定、等待缺失片段及重放规则时允许，并须写入能力报告。消息大小上限取实测支持值；超限先拒绝，不截断身份或关键结果。

## 6. 投递与恢复要求

- 同一 Delivery 的全部消息必须处理，不能因类型过滤漏掉旧消息。中心以 home runtime、Run 和 message ID 去重。
- 验证 check 的 FIFO 重放、ack 和 peek 行为，按实际版本完成其要求的处理后再 ack。未回答的问题若要求保留 Delivery，则持久保存该批次，并通过已验证的只读观察途径继续展示后续活动；不得用提前 ack 绕过协议要求。
- 测试消息入库后、ack 前中断；重连必须允许重放而不重复应用结果。停止或释放资源必须遵循目标 Dispatch 的实际 settlement 与恢复回执。
- 调用响应丢失时，仅使用平台已知且持久保存的 request ID 进行同请求恢复。若 request ID 也未取得，标记 `outcome_unknown`，只读对账或人工处理；空列表及暂时查不到不能证明动作未发生。
- capability 缺失、网络断开或身份不可验证时，不能回退到本机执行、替换目标 host 或盲目重发。
- 输出 cursor 绑定原 source；source_changed 时重新建立读取位置并记录缺口，不能把 cursor 当成跨 source 的通用序号。

## 7. 验收场景

| 编号 | 场景与通过条件 |
| --- | --- |
| P01 | 后台协调者启动远端 worker，返回实际远端身份；远端不存在 SteerLine 常驻进程 |
| P02 | 固定 base 在远端可解析并核验；中心私有 ref 和本机路径均未成为远端依赖 |
| P03 | worker 输出能增量读取；分页不漏已保留内容；缺口/source_changed 被显式报告 |
| P04 | ask 显示完整问题，超时恢复同一问题，reply 后取得应用证据，不产生第二次提问 |
| P05 | 成功/失败 worker_done 均可绑定原尝试；结构化结果不会因平台字段处理而丢失 |
| P06 | 断开网络后状态为不可验证；重连仍指向原 Dispatch，无第二个 worktree/worker |
| P07 | 中心进程在派发、收消息、回答期间退出；重启后可恢复或明确人工核实，不静默重发 |
| P08 | request ID 已取得和未取得两种响应丢失情况分别按恢复规则处理 |
| P09 | 资源释放只作用于已结算目标；worktree 和所需报告保留，不清理其他终端 |
| P10 | 旧版本缺失字段、未知 payload、超限消息、权限提示均有明确失败/降级，不伪造成功 |
| P11 | 直接脚本执行的输入、结果、退出和重连行为均有实测结论；失败项明确列为后续门槛 |
| P12 | 同一 worktree 先后执行至少两个受监管步骤，前一步释放不删除代码，下一步有新身份且无法误接旧结果 |
| P13 | 验证脚本返回非零、检查回执缺失或 commit 不符时，中心可识别未通过，不能仅凭 agent 摘要认定通过 |

必需能力的正向场景和故障场景全部有可核对证据，才能宣布远程 agent 协议可用于 Spec 03。当前对话中只检查过本机 CLI/指南，没有已配对的真实远端，此项状态保持待验证。
