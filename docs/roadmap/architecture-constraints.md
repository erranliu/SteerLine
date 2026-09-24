# 架构规划约束

适用于双链路控制台、中心状态机、远程执行，以及后续并行执行、共享服务和移动端的规划。此文用于约束规划；执行要求须落实到相应 Spec。

## 长期双执行路径

local Codex 与 Orca 均长期正式支持。backend、agent、execution target 和协议版本分开；本地路径不作为远程故障时的隐式替代。Orca terminal v2 与 supervised 可采用不同协议实现，共用业务边界和契约测试。

落地规格：[执行架构共同约束](../spec/execution-architecture.spec.md)、[双后端契约](../spec/pr-04a-execution-backends.spec.md)。

## 业务权威与执行边界

SteerLine 负责任务领取、流程、决策、恢复、PR 核验和结项；适配器负责执行现场及平台事实。UI/CLI 使用统一查询和命令接口。中心模型和存储演进应复用已有接口，不能产生竞争业务写者。

远端仅需 Orca、agent 和仓库工具，不要求部署 SteerLine 服务或共享中心磁盘。中心停止监督与取消执行分开；具体路径的生命周期策略明确表达。

落地规格：[共同约束](../spec/execution-architecture.spec.md)、[执行模型](../spec/02-execution-model-and-adapter.md)、[远程闭环](../spec/03-remote-execution-and-viewer.md)。

## 身份与扩展

Task、WorkflowRun、Execution、StepAttempt、Decision 和外部平台身份分开。任务领取与结项属于任务级；agent、工作区、输出和问题属于具体执行/尝试。单执行槽属于当前调度策略，不限制未来一个任务多个独立执行；并行执行与重试不混同。

历史操作固定到派发时的目标及代次，配置变化和迟到消息不能重定向原执行。远端路径属于其目标，不默认为本机文件。完成由中心按选用结果和真实交付核验。

落地规格：[执行模型](../spec/02-execution-model-and-adapter.md)、[远程协议](../spec/01-orca-remote-protocol.md)。

## 原生任务与人工处置边界

原生任务复用中心状态机与 SQLite，任务 DAG 决定任务间的执行资格，WorkflowRun/Execution/StepAttempt 承担任务内部执行。创建任务与加入 DAG 为同一事务，依赖和交付契约随创建固定；PR 合并前可受控丢弃，即使任务已执行，丢弃也不释放后继或抹去执行事实。已合并 PR 的任务不可丢弃。系统不提供任务拆分、回退、重新打开、动态依赖修改或级联补偿流程，也不引入父子聚合来承担人工拆分。

用户可通过 Codex 分析并人工实施拆分或回退，系统不承诺自动修复相关状态。重启恢复、对账与原契约下的重试继续支持。图算法、布局和 UI 均不成为业务写者。

共同要求：[原生任务共同约束](../spec/native-task-constraints.md)。独立执行规格：[任务模型](../spec/task-core.spec.md)、[固定 DAG](../spec/task-dag.spec.md)、[Beads 退出](../spec/beads-exit.spec.md)、[关系视图](../spec/dag-view.spec.md)；各有同名配套 Plan，每任务一个独立实现 PR。Beads 路径在对应项目按规格切换前继续有效，不据规划摘要修改当前任务权威。

## 证据、恢复与迁移

外部动作的意图、回执和不确定性可恢复；超时不能证明没有副作用，不能盲目重派。消息内容先持久化，再推进游标或确认接收。平台能力以具体版本/目标的原生证据为准。

新模型保留历史身份与数据，采用明确版本迁移；新结构不能复用含义不同的既有 schema 版本。共享部署与多进程演进继续保持事务或等价并发控制。

落地规格：[双后端契约](../spec/pr-04a-execution-backends.spec.md)、[会话事件](../spec/pr-04b-conversation-events.spec.md)、[执行模型](../spec/02-execution-model-and-adapter.md)。
