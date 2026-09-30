# Spec Orca 执行适配器迁移

任务 ID：58；任务名称：Orca 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：待实施。配套 [Plan](execution-isolation-05-orca.plan.md)。硬前置：[#57](execution-isolation-04-codex.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

将中心 Orca 资源、终端、派发、观察、消息和对账操作移入 `orcaworker`，遵守[共同约束](execution-isolation-constraints.md)。覆盖 native 以及已有 supervised 实现的所有者整理，不启用当前不可启动的路径，不增加另一种 agent 认证。

新增 `backend.go`、`config.go`、`events.go`，复用 workspace/terminal/orchestration/dispatch/message/relay/output 文件。主要迁移 `orca_worker_prepare.go`、`orca_worker_dispatch.go`、`orca_worker_observation.go`、`orca_worker_evidence.go`、`orca_worker_terminal.go`、`orca_worker_workspace.go`、`orca_worker_relay.go`、`orca.go`、`orca_supervised.go` 与 `execution_backend.go` 中的 Orca I/O 和证据解释。沿用原生 event/message_identity/saved_receipt 实现，必要 CLI 小输入归包，不能导入主包 Config/Runner。

## 资源与派发契约

中心提供固定项目、逻辑仓库、Execution、代次、目标与请求 ID。适配器核实 Repository、worktree、terminal/incarnation、coordinator、Run、Task、Dispatch 和 Delivery 各自身份，返回资源绑定与回执，不将这些 ID 相互替代或推定为 Codex UUID。

每个外部资源创建/派发/结算动作必须有中心已保存的固定意图。适配器可分步返回部分绑定或待对账事实；中心提交后再发下一请求。未知 Task/Dispatch 创建只查询原 request ID，不重复粘贴或另起 terminal。消息持久化并原子记录消费后才确认；处理尚未确认 Delivery 的重放继续使用 saved_receipt 核对来源，relay 路径绑定执行/轮次并保持既有授权和去重规则。

## 命令与 agent 隔离

Codex 命令来自 #57 的 executable/argv 契约，Orca 只做 shell 包装、终端创建、注入及原请求对账，删除原包内重复 Codex model/effort/sandbox/Git 参数生成。包装不得吞掉或重新解释 argv，不记录秘密参数。

Windows Orca Codex 已在未指定 model 时使用同一受管启动，#57 迁移参数生产者，本轮保持原行为；#59 只改变项目授权来源，不重新建设 managed launch。其他原有 agent 不假定具有 Codex 参数能力，不通过隐藏 CLI 映射改变其行为。supervised 的能力拒绝保持。

PR 线索使用 #54 的公共纯 helper，不为复用 PRClues 引入 codexworker 依赖。workspace 的本机 Git 操作与 Orca 原生工作区操作保持不同实现，用共同资源事实交接；不把中心本机目录当作远端实际位置。

## 观察与中心所有权

原生解析、terminal 等待/退出、transcript、来源缺口及 Dispatch 结算归适配器，返回共同事件与原始证据引用。业务状态转换、问题 Decision、PR 查询/绑定、Operation 提交和 TaskDelivery 归中心。

原 `orcaworker/environment.go`、`readiness.go` 的输入资格、HostFact、工具 wrapper/收据与安全文件校验继续归平台；`worker_environment.go` 的准备预算、轮次分流、Ready/Invalidated、DeferredDecision/Feedback、打开 terminal 和重查事务归中心。准备不写业务 CurrentTurn/计数，不消费业务反馈或 PR 线索；只有完整工具/收据与原生结算且无 blocker 才派发业务。

宿主菜单/父 shell 不接收 worker 文字；中心选项核对原 HostTarget、Decision 版本、blocker 和 terminal/incarnation。重查期间新故障不能被旧答案清除，重启不延长 nonce/deadline，确认旧轮失败/结算之前不创建新准备轮。原观察缺口和晚到成功保持当前保守处理，不重做 readiness 状态机。

实时、重放和恢复使用同一证据核验。观察失败不推断 worker 已停；旧 Dispatch、旧 incarnation 和迟到消息不推进当前轮次。恢复只接回原资源观察及对账，执行性继续由中心检查用户授权；清理只作用于原持有的精确资源，失败或未知仍返回可恢复事实。

## 开工与交接

可在 #54 契约固定后开展低层阅读、fixture 整理及隔离协议实验，但正式领取和完整实现须等待 #57 合入；不以准备工作绕过硬前置。共同中心文件、恢复入口与架构收口串行集成。

#59/#60 接收已完成的两类后端契约。至本轮结束，中心的 Orca 业务消费者不再读 native JSON 或根据 tail 文本猜业务结果。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| OC1 | orcaworker 不依赖中心/SQL/codexworker，中心共同流程不再解释 Orca 原生响应；Codex 命令只有一个生产者 |
| OC2 | 原 terminal/Dispatch 复用及固定配置回归通过，替换 incarnation、旧消息和迟到回执拒绝 |
| OC3 | 未知动作不重复；准备/业务轮、收据/工具/结算、固定期限与重查并发门槛保持，宿主动作与 worker 文字不串接 |
| OC4 | native 当前支持范围保持，已有 supervised fixture 通过且生产仍明确不可启动 |

复用既有 fake CLI、消息、relay 与命令 fixture，不因搬包要求真实跨机、完整 transcript 或人工 PR 合并链。
