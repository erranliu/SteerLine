# Spec Orca 执行适配器迁移

任务 ID：58；任务名称：Orca 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：待实施。配套 [Plan](execution-isolation-05-orca.plan.md)。硬前置：[#57](execution-isolation-04-codex.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

遵守[共同约束](execution-isolation-constraints.md)，迁移中心本机 Orca I/O/解释到 orcaworker，收敛远程中心桥到已有 orca_remote_worker 的固定契约。两条 Orca 持续链路已接通，保留能力与身份边界；不把远程代码重新揉入本机包，不增加新 agent 或远程取消能力。

本机包新增必要 backend/config/events，复用现有原生协议；迁移 orca_worker_prepare/dispatch/observation/evidence/terminal/workspace/relay 的平台部分。远程复用 orca_remote_worker 的 preflight/start/messaging/observation/run 与配置，收敛 orca_remote_adapter/service 和 orca_supervised 的平台桥。旧 shell turn、prompt/pull/journal、orcaexec 已删，不重新列为搬迁或测试目标。

## 资源与派发契约

中心提供固定项目、逻辑仓库、Execution、代次、目标与请求 ID。适配器核实 Repository、worktree、terminal/incarnation、coordinator、Run、Task、Dispatch 和 Delivery 各自身份，返回资源绑定与回执，不将这些 ID 相互替代或推定为 Codex UUID。

每个外部资源创建/派发/结算动作必须有中心已保存的固定意图。适配器可分步返回部分绑定或待对账事实；中心提交后再发下一请求。未知 Task/Dispatch 创建只查询原 request ID，不重复粘贴或另起 terminal。消息持久化并原子记录消费后才确认；处理尚未确认 Delivery 的重放继续使用 saved_receipt 核对来源，relay 路径绑定执行/轮次并保持既有授权和去重规则。

## 命令与 agent 隔离

Codex 命令来自 #57 的 executable/argv 契约，Orca 只做 shell 包装、终端创建、注入及原请求对账，删除原包内重复 Codex model/effort/sandbox/Git 参数生成。包装不得吞掉或重新解释 argv，不记录秘密参数。

该命令交接只适用于本机 native。远程原生 worker-start 固定连接/runtime/repository/agent/model/effort，后续原 terminal/incarnation 续聊，不生成中心 Codex 命令、不创建本地替代执行；远端路径按原值保存，不能按中心 OS 规范化。远端 worktree 保留，协调 inbox/Run 位于中心，两边身份分别验证。

Windows native 已在无 model 时受管，#57 迁移生产者；#59 改项目授权来源。远程仍要求显式 model/effort，保留原平台预检和能力拒绝；不把本机默认或准备流程泛化到远端。其他 agent 不假定具有 Codex 参数能力。

PR 线索使用 #54 的公共纯 helper，不为复用 PRClues 引入 codexworker 依赖。workspace 的本机 Git 操作与 Orca 原生工作区操作保持不同实现，用共同资源事实交接；不把中心本机目录当作远端实际位置。

## 观察与中心所有权

原生解析、terminal 等待/退出、transcript、来源缺口及 Dispatch 结算归适配器，返回共同事件与原始证据引用。业务状态转换、问题 Decision、PR 查询/绑定、Operation 提交和 TaskDelivery 归中心。

原 `orcaworker/environment.go`、`readiness.go` 的输入资格、HostFact、工具 wrapper/收据与安全文件校验继续归平台；`worker_environment.go` 的准备预算、轮次分流、Ready/Invalidated、DeferredDecision/Feedback、打开 terminal 和重查事务归中心。准备不写业务 CurrentTurn/计数，不消费业务反馈或 PR 线索；只有完整工具/收据与原生结算且无 blocker 才派发业务。

HostFact、探针/收据/工具证据和 WorkerEnvironment 的共享值已归 contracts，orcaworker 中现有别名是该记录的单一类型接线，不是第二套平台实现。本轮迁解析和操作，不重复定义这些结构，也不把中心 WorkerEnvironment 迁入平台包；contracts 保持不导入平台。

宿主菜单/父 shell 不接收 worker 文字；中心选项核对原 HostTarget、Decision 版本、blocker 和 terminal/incarnation。重查期间新故障不能被旧答案清除，重启不延长 nonce/deadline，确认旧轮失败/结算之前不创建新准备轮。原观察缺口和晚到成功保持当前保守处理，不重做 readiness 状态机。

实时、重放和恢复使用同一证据核验。观察失败不推断 worker 已停；旧 Dispatch、旧 incarnation 和迟到消息不推进当前轮次。恢复只接回原资源观察及对账，执行性继续由中心检查用户授权；清理只作用于原持有的精确资源，失败或未知仍返回可恢复事实。

远程启动/回复/ack 回执丢失按原 request、question history 和 Dispatch 对账，不重发未知输入。Attempt 初始绑定保持不可变，新 Dispatch 回执独立保存；进程观察用核实后的当前 Dispatch。合并交付不依赖 release 成功，release 仅作用于已结算 worker，中心协调 terminal 需排空且身份匹配。取消/停止远端仍 unsupported，停止监督不等于远端退出。

## 开工与交接

可在 #54 契约固定后开展低层阅读、fixture 整理及隔离协议实验，但正式领取和完整实现须等待 #57 合入；不以准备工作绕过硬前置。共同中心文件、恢复入口与架构收口串行集成。

#59/#60 接收已完成的两类后端契约。至本轮结束，中心的 Orca 业务消费者不再读 native JSON 或根据 tail 文本猜业务结果。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| OC1 | orcaworker 不依赖中心/SQL/codexworker，中心共同流程不再解释 Orca 原生响应；Codex 命令只有一个生产者 |
| OC2 | 原 terminal/Dispatch 复用及固定配置回归通过，替换 incarnation、旧消息和迟到回执拒绝 |
| OC3 | 未知动作不重复；准备/业务轮、收据/工具/结算、固定期限与重查并发门槛保持，宿主动作与 worker 文字不串接 |
| OC4 | 远程两轮/问题/丢失回执/替换身份与合并后资源结算回归通过；保留已接通链路、工作区与取消不支持，不宣称双机真机认证 |

复用本机和当前远程 Service/协议 fixture，不恢复旧 exec journal 测试。远程真实连接尚无本轮双机验收材料，代码/模拟回归不能冒充认证；本次搬迁不额外要求跨机或人工合并链。
