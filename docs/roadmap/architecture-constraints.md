# 架构规划约束

适用于双链路控制台、中心状态机、远程执行，以及后续并行执行、共享服务和移动端的规划。此文用于约束规划；执行要求须落实到相应 Spec。

## 长期双执行路径

local Codex 与 Orca 均长期正式支持。backend、agent、execution target 和协议版本分开；本地路径不作为远程故障时的隐式替代。Orca terminal v2 与 supervised 可采用不同协议实现，共用业务边界和契约测试。

落地规格：[执行架构共同约束](../spec/execution-architecture.spec.md)、[双后端契约](../spec/pr-04a-execution-backends.spec.md)。

## 业务权威与执行边界

按 #36 的目标边界，SteerLine 负责任务领取、工作区与 worker 关联、五个主阶段、完整反馈送达、原会话重接、PR 关联和真实合并结项。worker 负责实现、提交、创建 PR；测试、修复、审查由任务 Spec/Plan 约定。中心不再维护项目五步编排、检查清单、修复轮次或脏目录门槛。UI/CLI 使用同一 Query/Command，复用唯一业务写者。

远端仅需 Orca、agent 和仓库工具，不要求部署 SteerLine 服务或共享中心磁盘。中心停止监督与取消执行分开；具体路径的生命周期策略明确表达。

目标规格：[持续 worker 流程](../spec/worker-flow.spec.md)。既有[共同约束](../spec/execution-architecture.spec.md)、[执行模型](../spec/02-execution-model-and-adapter.md)与[远程闭环](../spec/03-remote-execution-and-viewer.md)的被替代范围按 #36 确定，当前实现不会因规划文字自动切换。

## 身份与扩展

Task、WorkflowRun、Execution、会话/轮次、用户消息和外部平台身份分别保留归属。任务领取与结项属于任务级；agent、工作区、输出和反馈属于具体会话。#36 移除按业务步骤派发的 StepAttempt 主链。每个 WorkflowRun 只有一条持续 worker 会话；后续多候选扩展为一个 Task 下多个独立 WorkflowRun，不为同一候选的反馈重建 worker，中心仍只记录一个 TaskDelivery。

历史操作固定到派发时的目标及代次，配置变化和迟到消息不能重定向原执行。远端路径属于其目标，不默认为本机文件。完成由中心按选用结果和真实交付核验。

落地规格：[执行模型](../spec/02-execution-model-and-adapter.md)、[远程协议](../spec/01-orca-remote-protocol.md)。

## 原生任务与人工处置边界

原生任务复用中心状态机与 SQLite，任务 DAG 决定任务间资格，WorkflowRun/Execution 管理持续会话及交付。创建任务与加入 DAG 为同一事务，交付契约随创建固定；未交付任务可按[追加前置 Spec](../spec/native-task-add-predecessors.spec.md)只增加依赖。PR 合并前可受控丢弃，但不释放后继或抹去执行事实。已合并 PR 的任务不可丢弃或补前置。新规划改变固定交付范围或要移除旧前置时，须重新规划受影响任务，不能用普通反馈或追加边改写旧契约。

用户可通过 Codex 分析并人工处置任务与成果，系统不承诺自动修复任意人工改动。当前格式重启、原请求对账和同会话继续必须支持。关系视图和 UI 均不成为业务写者。

共同要求：[原生任务共同约束](../spec/native-task-constraints.md)。独立执行规格：[任务模型](../spec/task-core.spec.md)、[固定 DAG](../spec/task-dag.spec.md)、[原生接入](../spec/beads-exit.spec.md)、[关系视图](../spec/dag-view.spec.md)；各有同名配套 Plan，每任务一个独立实现 PR。现有 Beads 代码只作为重构输入，不形成需要长期维护的生产兼容路径。

## 未发布版本的兼容策略

SteerLine 目前是未发布版本。默认不要求兼容先前开发版本的数据、数据库结构、配置或 SteerLine 自身协议；只有具体 Spec 明确列为交付要求时才保留兼容。上线前不为旧 runtime、旧数据库或 Beads 历史建设迁移和回退流程。检测到不支持的旧状态时阻止有副作用操作，不能通过静默重建掩盖可能仍在运行的外部执行。Orca 等外部平台只启用已实测的版本与能力。

## 证据与恢复

外部动作的意图、回执和不确定性可恢复；超时不能证明没有副作用，不能盲目重派。消息内容先持久化，再推进游标或确认接收。平台能力以具体版本/目标的原生证据为准。

当前格式内的重启、故障对账和数据完整性仍需验证。共享部署与多进程演进继续保持事务或等价并发控制。具体边界见[执行架构共同约束](../spec/execution-architecture.spec.md)。

落地规格：[双后端契约](../spec/pr-04a-execution-backends.spec.md)、[会话事件](../spec/pr-04b-conversation-events.spec.md)、[执行模型](../spec/02-execution-model-and-adapter.md)。
