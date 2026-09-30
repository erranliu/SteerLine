# Spec：架构收口 PR 1 — worker 协议与终态边界

状态：已由 #147 合入主干；未分配 Task ID。日期：2026-09-30。审计基线：`main@4e786d7`，包含已合并的 #136–#143。本系列按本文 → [PR 2](architecture-02-worker-commit.spec.md) → [PR 3](architecture-03-runtime-boundary.spec.md) 交付，每份 Spec 对应一个实现 PR；实施时从最新主干承接已合入改动。

## 1. 目标与依据

中心业务流程消费经过核验的 worker 事实；平台适配层解释各自的消息、日志、身份格式和回执。一次完成三种现有路径的事件与轮次终态整理：local Codex app-server、原生 Orca、受监督 Orca。准备、启动、反馈和恢复沿用固定 Execution 配置及现有 mode 路由。

遵守 [worker-flow](worker-flow.spec.md)、[原生 Orca](worker-flow-orca-native.spec.md)、[执行架构](execution-architecture.spec.md)中的当前流程、唯一业务写者、身份及交付约束。本文是现有能力的内部重构，不重启旧五步流程。项目尚未发布，不新增旧数据、旧配置或旧协议兼容路径。

## 2. 当前问题与交付范围

`workerFlowContext` 与 `workerFlowMode` 已固定配置和身份，`workerflow` 已承接部分纯规则；继续复用这些边界。当前 `eventWorkerFlowFor` 仍解析 app-server JSON，`finishWorkerFlowFor` 仍选择平台日志并解释终态。原生 Orca 在 `pollNativeOrcaWorker`、`finishNativeOrcaTurn` 中另有问题、结算和 PR 处理，必须一起纳入。

| 层 | 本 PR 的职责 |
| --- | --- |
| 平台适配层 | 读取/解析来源，核验原生身份、回执和结算，输出有类型的事件或终态证据；保留原始证据定位 |
| 中心 Service | 核对所属 Task/WorkflowRun/Execution/Attempt 与当前轮次；选择业务转换、持久化、调用 PR provider 和外部动作 |
| `workerflow` | 使用最小业务输入执行纯状态规则，不依赖父包、SQLite、平台 SDK、文件或进程 |

覆盖实时观察、重放和恢复读取的协议解释；恢复结果如何提交由后续 PR 迁移。此次保持现有持久化时序，接口收窄的最终清理由 PR 3 完成。

## 3. 内部事实契约

定义最小的有类型内部输入。名称和文件位置可由实现确定，但不能以通用 `map[string]any`、原始 JSON 或携带整个 `Runtime` 代替边界。

| 事实 | 必须能表达的内容 |
| --- | --- |
| 来源与关联 | 中心固定身份、平台来源标识、原生 request/turn/Dispatch/question 关联、原始证据引用以及当前事实是否已核实 |
| 启动与输入 | 输入已接受、轮次已开始分别表达；记录对应 command/request 和原轮次，不能相互推导 |
| 问题与回答 | 问题内容、选项、原问题身份、回答被接受或拒绝的证据；队列中的后续问题保持稳定关联 |
| 轮次结束 | 已核实成功、已核实失败、结果未明；最后回复、PR 候选、诊断和仍待核对的操作 |
| 输出观察 | 普通活动、来源切换、缺口和截断；展示事件不自动改变业务状态 |

各模式只提供平台实际存在的身份。Codex thread/turn/generation 校验留在 Codex 适配层；Orca Run/Task/Dispatch/terminal/incarnation 与消息来源校验留在 Orca 适配层。中心保留业务关联和过期判定，不要求所有 agent 提供 Codex UUID，也不把不同命名空间的 ID 当作同一字段使用。

平台专有的持久化字段和原始回执可以继续存在，由相应适配层解释。内部事实应包含中心转换真正需要的信息，不能迫使中心再次解码原始回执才能判断回答或轮次是否结束。

## 4. 处理与恢复规则

1. 固定上下文必须在读取或发送前确定；当前全局配置变化不能改变已有 Execution 的目标。适配层接受固定输入，不通过可写 `Service`/`Runtime` 引用修改业务记录。
2. 原始消息先由对应适配层核验，中心再根据当前业务身份消费。迟到的旧轮次消息可保留历史，但不能推进新轮次、替换原会话或重开已完成/丢弃任务。
3. 同一来源消息的实时处理和恢复重放共用解析与核验规则。重复问题、回答回执及完成消息不能产生第二个 Decision、外部发送或状态推进。
4. local 的 tool completion 必须匹配原问题和已保存答案；原生 Orca 的 `worker_done` 必须结合当前 Dispatch 的实际结算。输入接受、PTY 存活、最后一段文本或进程退出本身都不能充当成功终态。
5. 结果不足时保留原请求、会话及资源，输出未核实事实，由中心沿用现有待决策/对账处理。不得重建 worker、换后端或自动重发未知操作。
6. PR 候选提取归平台证据处理；仓库、分支、合并事实与唯一 TaskDelivery 判定归中心。多候选、关闭未合并、失败轮次中的 PR 线索遵循既有业务规则。
7. 适配层不得因解析成功提前 ack 或推进业务消费位置。沿用先持久化、后确认的顺序；普通会话输出的记录和缺口语义保持可追踪。

## 5. 实施落点

先定义事实类型和各模式的转换入口，再迁移 app-server 事件、日志终态以及 Orca question/reply/done 解析，最后接入中心共用转换。复用 `codexworker`、`orcaworker`、现有 app-server/Orca 适配文件与 `workerflow`；仅在确有依赖需要时新增包。

重点检查 `worker_start.go`、`orca_worker_observation.go`、`codex_worker_recovery.go`、`worker_resume.go` 和现有证据包。文件移动不是完成条件：中心的事件/终态业务路径不得再判断原生 method、解析平台 JSON、拼接平台日志路径或用平台文本格式推导业务事实。

准备和启动中的平台 I/O 继续由已有适配入口承接；不建设通用插件注册、事件总线或另一套工作流引擎。本 PR 不新增执行模式、公共 API、数据库格式或能力认证。

## 6. 最小验收

| 编号 | 必须验证的结果 |
| --- | --- |
| WP1 | 三种模式均经事实契约接入中心；原生 Orca 的独立观察/结束入口不存在第二份业务终态规则；纯规则包无平台及 I/O 依赖 |
| WP2 | 复用各模式现有 fixture，覆盖开始、问题、回答、结束的有效链；保留原生身份及原始证据；输入接受不被误判为开始或成功 |
| WP3 | 错误身份、旧 Dispatch/turn 和重复消息不改变当前业务状态，也不重复发送；已结束任务不被迟到回执重开 |
| WP4 | 缺少终态、回答证据不匹配以及多个 PR 候选保持现有保守处理；恢复读取与实时处理对同一证据得出相同事实 |
| WP5 | 现有固定配置路由、local 两轮反馈及原生 Orca 原 terminal 复用的针对性回归通过 |

优先复用 `TestAppServerServiceTwoTurnsPersistNativeReceipts`、`TestAppServerAnswerEvidenceRequiresMatchingNativeContent`、`TestAppServerOutcomeUsesNativeTurnAndEarlierPRItem`、`TestNativeOrcaRejectsLateDispatchMessages` 和现有受监督 Orca fixture，只为缺少的接线/事实边界补例；同一场景可覆盖多项要求。只运行改动包和受影响用例，记录命令、结果及未覆盖范围；不默认要求全量套件、真实跨机环境或重新走人工 PR 合并链。

## 7. 交付与后继

PR 描述列出三种模式的入口、中心消费者及测试映射。合入后中心使用统一事实驱动现有业务转换，平台解析仍只有一份实现。[PR 2](architecture-02-worker-commit.spec.md) 在此基础上更换业务提交方式，不再重新设计消息或终态协议。
