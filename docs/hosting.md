# Hosting 红线：中心管业务，执行端管现场

SteerLine 同时支持本机 Codex 和 Orca 执行。当前 Orca terminal v2 实现使用 worktree、终端和结果文件；受监管远程路径使用精确 Run/Task/Dispatch/host/worktree、消息 Delivery 与 worker_done，按 [执行架构 Spec](spec/execution-architecture.spec.md)、[远程协议 Spec](spec/01-orca-remote-protocol.md)与[远程闭环 Spec](spec/03-remote-execution-and-viewer.md)实施。隔离真机已验证 prepare/implement/verify、单问题问答、R06 双故障、若干中心重启窗口及 PR/合并的独立对账；生产能力报告仍保持未验证，claim 前拒绝。当前实现的细节见 [Hosting v2 规范](spec/hosting-spec.md)，不作为目标远程路径的限制。

原生任务接入前，现有 Beads 任务源与 Orca terminal v2 可以作为过渡实现和验证路径；这不形成长期的双任务权威或旧数据兼容要求。以下红线描述目标架构及过渡中不能破坏的执行安全边界。

## 权威边界

| 事实或动作 | 权威 |
| --- | --- |
| 业务 Task、固定 DAG、领取、步骤顺序、决策状态、恢复、交付和结项 | SteerLine 中心服务 |
| worktree、agent、Orca Run/Task/Dispatch、平台消息和执行存活事实 | 对应的本机或 Orca 执行后端；中心按固定身份核验并保存事实 |
| PR 身份、head、目标分支与合并事实 | PR 平台；中心重新查询并核验 |
| PR 审阅与合并、越权或证据不足时的取舍 | 人 |

Orca 的 Run/Task/Dispatch 是执行身份，不等同于 SteerLine 的业务 Task、WorkflowRun 或 DAG。可以通过 Orca 的受监管 worker 和 ask/reply 协议执行固定意图；不能让 Orca 的 gate 或任务状态替代中心的业务门槛、决策与结项。UI、CLI 和 agent 均不能绕过中心命令直接写业务状态。

## 派发与结果

- 中心先持久化任务/步骤身份、固定仓库与 base、执行目标和外部动作意图，再调用后端；实际 worktree、Dispatch 和回执由执行端提供。远端路径属于远端，中心不能按本机路径读取远端文件或默认共享磁盘，远端也无需部署 SteerLine 服务。
- 后端失联、请求响应丢失或执行可能仍活跃时保留原身份与资源占用，先对账，不盲目重派，也不自动切到另一后端。后端适配器只返回平台事实，不领取或关闭业务任务。
- 中心按 implement、verify、按配置 review、publish_pr 的固定顺序推进。agent、技能和脚本只完成被派发步骤；自然语言总结、worker_done 或结果文件本身均不能证明验证通过或任务完成。验证回执、代码 revision、PR head 和真实合并事实须由中心分别核验。
- worker 的业务问题绑定原 Execution、Attempt 和 decision ID；答案保存、平台接受和实际应用分别记录。登录、信任目录与宿主权限提示交人处理，不伪装成业务问题。

## 生命周期与能力

- 关闭 viewer 只结束观察。关闭 Windows 服务窗口会停止中心监督；远端 worker 可能继续，不能因此断言已停止或释放资源。
- 当前受监管远程路径不提供生产取消；取消请求明确返回不支持并提示人工处理，不发送未经验证的 stop，也不凭 terminal handle 兜底关闭。已结算执行按后端协议释放资源，默认保留工作区与产物。
- 仅启用对应目标与版本组合已实测的能力。首版远程版本、消息上限和剩余验收以远程 Spec 为准；未知能力保持不可用，不用提示词补足平台证据。

## 当前 PR 4A 执行后端

当前 PR 4A 的本机执行接入：local Codex 和 Orca terminal v2 共用 Service 派发与业务状态机，分别通过适配器提供固定执行身份、工作区事实、原始输出、观察与停止。runtime v5 是当前 SQLite 数据库的读投影；不可变 binding 和后端 Operation 保存在 `steerline.db`，`execution/v1/` 只保存能力证据。v1–v4 与旧文件式执行状态不自动迁移。Orca agent 先等待派发，适配器取得真实 worktree/terminal 身份并写好提示文件后才向同一 terminal 发送正式指令。精确 PTY 停止证明必须先落盘，才能释放执行槽。

领取前按目标、agent、协议及 CLI 版本核对已验证能力。隔离原生验收后停服运行 `steerline certify-backend -config <config.json> -evidence <evidence.json>` 导入证据；未验证能力不可用于副作用。具体结果和仍 unsupported 的能力见 [PR 4A 验证报告](validation/pr-04a-execution-backends.md)。

PR 4B 的会话采集由服务持有，不依赖 viewer/TUI。Local Codex 原始 JSONL 在 worker 运行时和服务重启回放时进入同一 normalizer/SQLite store。Orca terminal v2 由固定 terminal/incarnation 的观察 collector 读取有界 cursor 页；原始终端文本保存为 observation，并附来源/cursor/gap。关闭 viewer 不影响服务采集；关闭 Windows 服务窗口会停止服务。Local JSONL 可从持久来源 checkpoint 对账回放。Orca 当前不自动采用旧 terminal observer；新 continuation observer 用新 segment 并记录缺少前一 cursor 的 gap。服务停机期间无法确认的来源历史保持 gap/unknown。

当前 terminal v2 文本观察不声明 turn completion 或 final phase。配置的 Codex 原生命令支持 app-server 时，本机 Orca 启动器在终端环境中显式预建、绑定 session，并保留 Orca 的 CODEX_HOME；服务通过该 session 的公开历史接口采集精确轮次、阶段与正文。该读取路径不创建 supervised Dispatch，也不读取私有数据库或远端路径。回复回执仍只证明输入接受和 turn start；结构化结果摘要单列为 `execution.result`。真实能力证据与未验收项见 [PR 4B 验证报告](validation/pr-04b-conversation-events-2026-09-24.md)。

## 审查红线

- 业务 DAG、步骤门槛、Decision 或交付事实是否被 Orca、agent、技能、UI 或脚本直接推进？
- 外部动作不确定时，是否启动了替代 worker、切换后端或释放了可能仍活跃的资源？
- 是否把结果文件、worker_done、PR 已创建或 PR closed 当成已核验交付？
- 是否把远端路径当成本机文件，或把服务/界面关闭当成远端执行停止？
