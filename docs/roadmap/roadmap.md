# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，已规划 Spec 的执行依赖、接口和并行条件见 [execution-map.md](execution-map.md)。

## 近期：稳定的双链路执行控制台

按以下依赖顺序推进，先完成前置能力与验证，再进入下一步。

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 0 | 双执行后端 | `EXEC-BACKENDS` | 将 local Codex 与 Orca 接入共同操作、固定身份、能力与故障恢复契约，保持长期支持 | 已交付的服务窗口生命周期与服务控制协议 | [Spec](../spec/pr-04a-execution-backends.spec.md) · [Plan](../spec/pr-04a-execution-backends.plan.md) |
| 1 | 统一会话事件与完整回复 | `CONVERSATION` | 统一会话/轮次/消息、完整回复、持久读取与重放；验证两条链路真实的轮次和阶段能力 | `#0` | [Spec](../spec/pr-04b-conversation-events.spec.md) · [Plan](../spec/pr-04b-conversation-events.plan.md) |
| 2 | 执行控制台与轮次折叠 | `CONSOLE` | 多项目操作与运行详情；轮次结束自动折叠过程，保留最终回复、未解决问题和阅读位置 | `#1` | [Spec](../spec/pr-04-console.spec.md) · [Plan](../spec/pr-04-console.plan.md) |
| 3 | 人工决策与恢复闭环 | `INTERVENTION` | 决策提交、答案应用证明、在线恢复与审计闭环，共用双后端接口 | `#2` | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) |

两条链路的精确轮次/最终回复能力需要真实来源证据。公开协议存在缺口时，将协议补齐作为相应功能的前置；执行级展示不能替代精确体验的验证。

### 后续仍依赖的已有基础

- [统一观察契约](../spec/pr-01-observation.spec.md)：供 `#1`/`#2` 复用选择、游标、来源换代及原始输出读取规则。
- [服务控制协议](../spec/pr-02-control.spec.md)：供 `#0`/`#2`/`#3` 复用请求身份、回执、准入及不确定动作守卫。
- [服务窗口生命周期](../spec/pr-03-service-lifecycle.spec.md)：供 `#0`/`#2` 复用连接分类、startup/instance 身份和停止确认；Windows 离线时提示用户从固定入口打开可见服务窗口，默认 manual，关闭 viewer 不停服，关闭服务窗口则结束服务。Linux detached 路径保留，原生会话生命周期尚未实测。

## 后续：中心状态机与远程执行

| 任务 ID | 任务名称 | 目标标识 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 4 | Orca 远程协议验证 | `REMOTE-PROTOCOL` | 收口 P10 消息大小与宿主权限提示的未测边界；结合 `#6` 接入完成 P13 中心拒绝无效验证结果的端到端验收 | 复用真实远端证据；补测使用隔离环境；仅支持已验证版本组合 | [Spec](../spec/01-orca-remote-protocol.md) · [Plan](../spec/01-orca-remote-protocol.plan.md) · [剩余门槛](../spec/orca-probe/remaining-gates.md) |
| 5 | 中心执行模型 | `EXEC-MODEL` | 沿用已分配任务 ID，引入 WorkflowRun/Execution/StepAttempt、最小流程状态机与 SQLite 事务；旧状态拒绝启动，不做数据迁移 | `#4` 已有的实测协议映射及能力限制；复用共同后端与会话契约 | [Spec](../spec/02-execution-model-and-adapter.md) · [Plan](../spec/02-execution-model-and-adapter.plan.md) |
| 6 | 分阶段远程闭环 | `REMOTE-WORKFLOW` | 中心逐步派发实现、验证、审查与发布；接入已有 verify 校验器，完成远程观察、人工决策、断线对账及结项 | 开发使用 `#4` 已实测协议及 `#5` 中心模型接口；生产启用前通过 `#4` 剩余门槛、`#5` 交付与本任务验收 | [Spec](../spec/03-remote-execution-and-viewer.md) · [Plan](../spec/03-remote-execution-and-viewer.plan.md) |

可依据实测证据开发受限远程适配器，协议目标保留剩余验收；P13 需要在中心接入后验证，不能将纯校验器测试视为端到端通过。剩余远端探测可与不依赖其结果的本机工作并行；中心模型的独立设计与本地存储测试不必等待远端环境。控制台与中心模型交叉使用同一执行/会话契约，后落地的工作扩展已有权威实现，避免形成两套适配器或业务写者。

首版远程范围仅支持已测 Windows Orca 1.4.209 中心/远端组合，其他版本预检拒绝；结构化消息采用不高于 8 KiB 的序列化大小上限，超限发送前拒绝，宿主权限提示无法可靠分类时交人工核实。生产远程取消不可用，中心保留活跃执行身份与资源占用并要求人工处理，不自动发送 stop 或按终端句柄兜底关闭；已结算目标的正常 release 仍可用。

### 后续仍依赖的已有基础

- [真实远端能力报告](../spec/orca-probe/remote-capability-report-2026-09-23.json)与[字段映射](../spec/orca-probe/field-mapping.md)：供中心模型和远程闭环复用派发、身份、输出、问答、同工作区跨步骤与故障恢复证据；会话目标可复用输出来源与缺口映射，仍须单独验证精确 turn/phase 能力。
- [verify 结果校验器](../../internal/steerline/remoteprotocol/verify_result.go)：供远程闭环接入已接受的 worker_done 身份、预期检查清单和执行端 HEAD，阻止缺回执、非零退出、身份或候选 commit 不符的结果推进审查/发布。

## 后续：原生任务与固定 DAG

SteerLine 自主管理任务和依赖，解除 Beads 必需依赖。原生任务创建时即入 DAG 并固定交付契约与外部依赖；PR 合并前可受控丢弃，已合并后不可丢弃。拆分与回退由用户通过 Codex 分析并人工处理，系统不提供对应流程。按下列顺序交付，每个任务各有独立 Spec、配套 Plan 和一个独立实现 PR；共同要求见 [原生任务共同约束](../spec/native-task-constraints.md)。

| 任务 ID | 任务名称 | 目标标识 | 剩余交付范围 | 关键前置 | 执行规格 |
| --- | --- | --- | --- | --- | --- |
| 7 | 原生任务模型 | `TASK-CORE` | 复用已分配任务 ID，交付固定规格契约、SQLite 扩展与中心查询 | `#5` 的身份、事务和 Query/Command 权威 | [Spec](../spec/task-core.spec.md) · [Plan](../spec/task-core.plan.md) |
| 8 | 固定依赖调度 | `TASK-DAG` | 批次原子创建入图、未合并任务丢弃、硬前置、事务领取、交付释放和重启恢复 | `#7` 验收并合入；复用既有执行与 PR 核验 | [Spec](../spec/task-dag.spec.md) · [Plan](../spec/task-dag.plan.md) |
| 9 | 解除 Beads 依赖与原生接入 | `BEADS-EXIT` | 原生配置/技能/worker/结项路径，交付无 Beads 新项目闭环；不导入旧项目数据 | `#8` 验收并合入；项目接入契约 | [Spec](../spec/beads-exit.spec.md) · [Plan](../spec/beads-exit.plan.md) |
| 10 | 任务关系视图 | `DAG-VIEW` | 任务上下游与阻塞说明、DOT/JSON 导出和可选 SVG 展示 | `#8` 的图查询与 `#2` 查询边界；`#9` 验收并合入后实施 | [Spec](../spec/dag-view.spec.md) · [Plan](../spec/dag-view.plan.md) |

首版只提供同项目的“前置交付后开工”依赖，沿用现有执行槽策略，不引入父子聚合、条件边或复杂并行调度。图算法与布局复用开源组件，任务权威保持在中心服务。图形展示先以只读导出交付，交互式 Web 视图另行规划；原生任务不等待远程闭环全部完成，也不扩大远端能力。

## 规划约束

- [架构约束](architecture-constraints.md)
- [控制台约束](console-constraints.md)
