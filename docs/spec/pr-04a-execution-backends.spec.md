# PR 4A Spec：长期双执行后端契约

任务 ID：0；任务名称：双执行后端。

状态：待实现。前置 PR 3 已验收并合并。对应 [Plan](pr-04a-execution-backends.plan.md)。依据：[总览](console-upgrade.md)、[执行架构共同约束](execution-architecture.spec.md)。

## B1：职责与长期支持边界

local Codex 与 Orca 是长期正式支持的执行路径。backend（local / Orca）、protocol（local_process / orca_terminal_v2 / 未来 orca_supervised）、agent 和 execution target 分开表达。配置使用本轮确定的格式；首版本地 agent 只需支持 Codex。local 不作为 Orca 故障时的隐式 fallback。

Service 唯一决定任务领取、准入、执行槽、业务状态转换、重试资格、PR 核验和结项。适配器负责平台执行、工作区与观察事实，不接收可修改业务状态的 Run 指针，不领取/关闭任务，不自行重试或写 runtime。Query/Command 层和 UI 依赖公共 DTO，不直接调用 Codex 或 Orca CLI。

分成三个边界：Service 业务规则 → ExecutionBackend 操作与事实 → agent 输出解码器。Codex JSONL 的 phase/turn 解析属于输出解码器；Orca 路由、终端/Dispatch 身份属于平台适配器。公共接口不能要求所有后端都提供 PID 或 terminal，也不能让共享业务代码依赖 Orca 原始响应结构。

本阶段接入本机 local / Orca terminal v2；不增加 SQLite、minimal-v1、多执行调度或受监管远端生产支持。[Spec 02](02-execution-model-and-adapter.md) 负责中心模型/存储，[Spec 03](03-remote-execution-and-viewer.md) 负责远端接入；若这些已先落地，复用其权威接口，禁止平行实现另一套 adapter 或业务写者。

## B2：固定输入与身份

- AttemptSpec 是不可变输入，至少含 project/task/run 或 workflow/execution/attempt 身份、操作 ID、固定配置摘要、仓库/base、工作区要求、agent、协议和目标。当前 Run 映射单个 execution，不在公共接口固化一个 Task 只能有一个 execution。
- ExecutionRef 保存 backend/protocol、固定 target/connection、agent、工作区与外部身份及代次。local 使用 PID 加可验证创建身份；Orca terminal v2 使用所属 runtime/worktree/terminal/incarnation；supervised 使用原 home runtime/Run/Task/Dispatch，不要求虚构 terminal。
- 启动意图、固定绑定和外部回执持久化后才可用作后续操作依据。配置默认值改变不能把历史运行路由到新机器、另一协议或另一 agent。连接凭证可通过同一固定连接身份更新，但不能把路由静默替换；记录诊断时不保存明文凭证。
- 当前格式的执行绑定必须有可核验身份；缺失或损坏时阻止有副作用动作，不从配置补造平台事实。不读取或迁移旧 runtime 格式。

## B3：共同操作契约

以下是语义接口，具体 Go 签名由实现确定；操作可拆成小接口，禁止为可选能力提供“空实现成功”。

| 操作 | 必需语义 |
| --- | --- |
| Probe / Preflight | 固定 backend/protocol/target/agent/version 下的能力与依赖检查；领取前尽可能发现缺失条件 |
| Prepare / Start | 固定输入和 Operation；返回真实 workspace/ref、accepted/rejected/outcome_unknown、已产生的残留资源；accepted 不等于 agent 已运行 |
| Observe | 原 ref 的 liveness、connection、等待、时间与来源；失联/权限不足/身份冲突与已退出分开 |
| InspectWorkspace | 固定 ref 下的仓库/worktree/base/HEAD/dirty 只读证据与采样时间；远端检查由目标适配器执行，不把远端路径交给本地 Git |
| ReadOutput | ref + 来源绑定 opaque cursor → 有界内容、next cursor、保留范围、gap/source_changed；语义遵循 [4B](pr-04b-conversation-events.spec.md) |
| Reply / Continue | 原问题/会话/attempt、固定答案、Operation；回执分别表达输入接受、轮次开始、应用证明；不自行推进 Decision |
| StopExecution | 取消精确执行；accepted/confirmed/rejected/outcome_unknown；confirmed 必须有原身份停止证据 |
| DetachObservation | 取消并等待本地观察任务退出，不发送取消执行，不删除资源；可重复调用 |
| SettleResources | 仅对已结算执行按 release/明确 retain 策略结算执行句柄；返回残留及结果，默认保留工作区和产物 |
| ReconcileOperation | 原 Operation/平台 request ID 的查询或已验证协议允许的同请求恢复；不重起替代执行 |

若平台把准备与启动合为一个动作，可以一次实现；仍须记录准备意图和实际资源，不能填期望路径/base 冒充已核验结果。Receive/Ack 是未来 supervised 的可选消息接口；当前无 inbox 的后端明确不支持，不制造空 Delivery。业务消息的 Ack 必须在持久化后进行。

每个有副作用动作前保存 Operation（ID、目标、固定负载摘要、状态）；相同 ID 同内容恢复原结果，不同内容拒绝。已发送却响应丢失、进程崩溃或平台身份未返回时保留 outcome_unknown 与资源许可。只有明确“未发送且无副作用”的证据允许重新尝试发送。平台支持请求去重/查询时按原身份恢复；本地 Operation ID 本身不提供跨系统 exactly-once 保证。

错误使用类型表达：unsupported、rejected、unreachable、identity_mismatch、protocol_invalid、outcome_unknown，并附诊断；业务代码不能匹配错误字符串决定重发。慢操作有超时/取消，单个 ref 的观察不重叠；停止和用户命令不能被无限输出或观察阻塞。共享平台客户端不得把一个 ref 的取消传播给其他 ref。

## B4：能力和策略

能力表按具体版本组合和目标保存 checked_at、证据引用与 verified / unsupported / not_tested。静态平台声明不能直接升级为 verified；目标、agent、协议或相关版本变化后需重新检查，历史报告保留。not_tested 不能启用尚未验证的有副作用能力。

至少分别表示 start、observe、stop、reply/continue、retry、operation reconciliation、workspace inspection、输出读取以及 4B 的独立输出能力。Service 在执行边界复核能力与业务资格；UI 只展示结果，隐藏按钮不能代替服务端守卫。预检按目标执行位置及任务来源检查工具；本机 Codex 不能成为 Orca agent 的无条件依赖。

既有能力范围保持：local 受限 retry；Orca 原终端/同 incarnation continuation；local continuation、Orca retry 首版 unsupported。能力为 verified 也不能绕过 dirty/base/claim/PR/unknown 等业务守卫。决定补齐可选能力时，必须增加对应原生验收，不能把新起 agent 当作恢复原会话。

生命周期策略在 Service 明确选择并随执行绑定：当前本机服务遵守 PR 3 停服等待、超时终止 worker 的规则。Windows 的服务由独立可见窗口承载；关闭 viewer/TUI 只结束客户端观察，窗口保持打开时服务和 worker 继续运行，关闭服务窗口则触发服务退出与本机 worker 清理。控制 stop、Ctrl+C 和关窗不能共用伪造的 applied 证明；离线 `StartService` 返回 `window-required` 与固定入口，不从客户端创建后台子进程。Linux detached 路径保持平台独立。未来远端遵守中心停止观察、执行可继续的策略。StopExecution、DetachObservation、SettleResources 分开调用，不能把 local Job 的 kill-on-close 或 Orca Close 的监听取消包装成相同的“释放即成功”。Stop 出错不得吞掉，未确认退出继续保留不确定性和槽约束。

## B5：存储与恢复

优先复用当前持锁 Service/维护入口；仅观察 goroutine 返回事实，由持锁写者应用。无 SQLite 时新增独立版本化绑定/Operation 记录及恢复索引，必须定义落盘顺序：意图 → 外部调用 → 绑定/结果 → runtime 投影；后两者之间失败可按前者重建，不能再次启动。文件原子替换失败、残留临时文件与磁盘满不视为成功。

本阶段确定唯一当前存储格式，不建设 runtime v1–v4 或旧数据库读取器。新记录和 runtime 的引用关系须在重启时对账，损坏/矛盾阻止新副作用。已落地 SQLite 时使用其事务/outbox，不再创建竞争文件权威。不要同时双写两套可独立恢复的状态。

## 验收

同一参数化契约套件运行 local 与 Orca 实现；平台替身测试用于故障注入，真实平台证据用于声明已验证能力，两者分别记录。

| ID | 场景与通过条件 |
| --- | --- |
| B-A1 | 两后端经过同一 Service 启动路径；claim/槽/结果/PR 规则唯一，业务层无直接平台启动分支 |
| B-A2 | success、明确拒绝、发送后断连、回执保存失败分别处理；未知动作重启不创建第二个 worker/worktree |
| B-A3 | 改默认 host/agent/连接后操作旧运行仍指向固定 ref；PID 重用、terminal incarnation 替换被拒绝 |
| B-A4 | Stop 接受但未退出、失败、超时均不伪确认；Detach 不取消，重复 Detach 可安全退出观察 |
| B-A5 | 两边检查各自所需工具和能力；unsupported/not_tested、当前绑定缺身份在实际调用前拒绝 |
| B-A6 | 较早 attempt 的迟到事实只进入其记录；不关闭其他任务、不解锁另一执行、不丢 claim/产物 |
| B-A7 | local retry 与 Orca 同会话续跑已有安全资格保持；无跨后端 fallback 或未知执行自动替换 |
| B-A8 | 意图/绑定/投影各持久化边界故障后可对账；损坏或不支持的旧状态阻止副作用，不自动迁移 |
| B-A9 | 取消、慢观察、大输出不阻塞停止与控制；观察任务结束后无泄漏或跨 ref 取消 |
| B-A10 | 真实 local Codex 和本机 Orca 的启动、输出、停止、重连证据归档；Windows 在真实服务窗口中验证客户端退出/重连时身份不变、关窗后本机 worker 清理及控制 stop 的原实例证明，Linux 行为另行记录；PR 3 生命周期回归通过 |

必需验收不能以 skipped 计为通过。平台能力缺口记录为阻塞或明确的可选 unsupported；不能以“接口已经统一”宣告未验证的路径稳定。
