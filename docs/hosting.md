# Hosting 红线：中心管业务，执行端管现场

SteerLine 的任务与交付只使用中心 native 权威。当前正式服务仅派发 `worker-flow-v1`，由同一 worker 会话处理实现、反馈和 PR 关联。第一阶段仅交付/验收本地 Codex 与本机 Orca。存量远端适配/探针和历史试验保留为后续资料，不属于一期派发、恢复、取消流程或完成门槛。

## 权威边界

| 事实或动作 | 权威 |
| --- | --- |
| 业务 Task、固定 DAG、领取、步骤顺序、决策状态、恢复和交付 | SteerLine 中心服务 |
| worktree、agent、Orca Run/Task/Dispatch、平台消息和执行存活事实 | 对应的本机或 Orca 执行后端；中心按固定身份核验并保存事实 |
| PR 身份、head、目标分支与合并事实 | PR 平台；中心重新查询并核验 |
| PR 审阅与合并、越权或证据不足时的取舍 | 人 |

Orca 的 Run/Task/Dispatch 是执行身份，不等同于 SteerLine 的业务 Task、WorkflowRun 或 DAG。可以通过 Orca 的受监管 worker 和 ask/reply 协议执行固定意图；不能让 Orca 的 gate 或任务状态替代中心的业务门槛、决策与结项。UI、CLI 和 agent 均不能绕过中心命令直接写业务状态。

## 派发与结果

- 中心先持久化任务/步骤身份、固定仓库与 base、执行目标和外部动作意图，再调用后端；实际 worktree、Dispatch 和回执由执行端提供。本期仅作用于本机工作区和 worker；保留远端路径的身份保护不变，不启用远端流程。
- 后端失联、请求响应丢失或执行可能仍活跃时保留原身份与资源占用，先对账，不盲目重派，也不自动切到另一后端。后端适配器只返回平台事实，不领取或关闭业务任务。
- 中心固定任务、工作区、会话和 PR 身份，worker 在原会话中持续工作。自然语言总结、worker_done 或 PR 已创建均不能证明任务完成；中心仍须核验真实合并事实。
- worker 的业务问题绑定原 Execution、Attempt 和 decision ID；答案保存、平台接受和实际应用分别记录。登录、信任目录与宿主权限提示交人处理，不伪装成业务问题。

## 生命周期与能力

- 关闭 viewer 只结束观察。关闭 Windows 服务窗口会停止中心监督；已有 worker 可能继续，不能因此断言已停止或释放资源。
- 本机停止/取消以目标实际支持和原 Operation 回执为准，不发送未经验证的 stop，也不凭 terminal handle 兜底关闭。保留远程路径不在一期范围，不能以停中心声明远端已退出。已结算执行按后端协议释放资源，默认保留工作区与产物。
- 仅启用对应目标与版本组合已实测的能力。本期只以本机目标和构建的实际证据为准；未知能力保持不可用，不用提示词补足平台证据。

## 历史 PR 4A/4B 执行后端记录

以下为旧验证时的路径、格式和能力记录，不作为当前一期支持或门槛；当前数据合同与本机范围见[数据合同](data-contract.md)。

当前本机执行接入：local Codex 和 Orca terminal v2 共用 native Service 派发与步骤状态机，分别通过适配器提供固定执行身份、工作区事实、原始输出、观察与停止。runtime v5 是当前 SQLite 数据库的读投影；不可变 binding 和后端 Operation 保存在 `steerline.db`，`execution/v1/` 只保存能力证据。v1–v4 与旧文件式执行状态不自动迁移。Orca 使用中心准备并核验的 worktree；固定步骤 launcher 与其摘要先持久化，之后才创建绑定到该 worktree 的 terminal。精确终端/进程身份停止证明必须按 Operation 对账，才能释放执行槽。

领取前按目标、agent、协议及 CLI 版本核对已验证能力。隔离原生验收后停服运行 `steerline certify-backend -config <config.json> -evidence <evidence.json>` 导入证据；未验证能力不可用于副作用。具体结果和仍 unsupported 的能力见 [PR 4A 验证报告](validation/pr-04a-execution-backends.md)。

PR 4B 的会话采集由服务持有，不依赖 viewer/TUI。Local Codex 原始 JSONL 在 worker 运行时和服务重启回放时进入同一 normalizer/SQLite store。Orca terminal v2 由固定 terminal/incarnation 的观察 collector 读取有界 cursor 页；原始终端文本保存为 observation，并附来源/cursor/gap。关闭 viewer 不影响服务采集；关闭 Windows 服务窗口会停止服务。Local JSONL 可从持久来源 checkpoint 对账回放。Orca 当前不自动采用旧 terminal observer；新 continuation observer 用新 segment 并记录缺少前一 cursor 的 gap。服务停机期间无法确认的来源历史保持 gap/unknown。

当前 terminal v2 文本观察不声明 turn completion 或 final phase。配置的 Codex 原生命令支持 app-server 时，本机 Orca 启动器在终端环境中显式预建、绑定 session，并保留 Orca 的 CODEX_HOME；服务通过该 session 的公开历史接口采集精确轮次、阶段与正文。该读取路径不创建 supervised Dispatch，也不读取私有数据库或远端路径。会话事件用于观察；native step 的结构化回执仍单独绑定 project/task/workflow/execution/attempt 并经过中心门槛校验。真实能力证据与未验收项见 [PR 4B 验证报告](validation/pr-04b-conversation-events-2026-09-24.md)。

## 审查红线

- 业务 DAG、步骤门槛、Decision 或交付事实是否被 Orca、agent、技能、UI 或脚本直接推进？
- 外部动作不确定时，是否启动了替代 worker、切换后端或释放了可能仍活跃的资源？
- 是否把结果文件、worker_done、PR 已创建或 PR closed 当成已核验交付？
- 是否把远端路径当成本机文件，或把服务/界面关闭当成远端执行停止？
