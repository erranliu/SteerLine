# SteerLine Roadmap

产品目标是以 roadmap 为核心的自动化软件开发系统。这里组织选定推进的目标；更广的后续产品计划见 [product-vision.md](product-vision.md)，目标的接口、冲突和并行条件见 [execution-map.md](execution-map.md)。

## 近期：稳定的双链路执行控制台

按以下依赖顺序推进，先完成前置能力与验证，再进入下一步。

| 步骤 | 剩余目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- |
| 后台服务生命周期（`SERVICE-LIFECYCLE`） | 完成独立启动、连接、停止与恢复的交付；补齐原生 Windows 下关闭启动端/真实终端后服务和 worker 存活、并发启动及显式停止的验证 | 服务控制协议 | [Spec](../spec/pr-03-service-lifecycle.spec.md) · [Plan](../spec/pr-03-service-lifecycle.plan.md) |
| 4A 双执行后端（`EXEC-BACKENDS`） | 将 local Codex 与 Orca 接入共同操作、固定身份、能力与故障恢复契约，保持长期支持 | 后台服务生命周期 | [Spec](../spec/pr-04a-execution-backends.spec.md) · [Plan](../spec/pr-04a-execution-backends.plan.md) |
| 4B 会话事件（`CONVERSATION`） | 统一会话/轮次/消息、完整回复、持久读取与重放；验证两条链路真实的轮次和阶段能力 | 4A | [Spec](../spec/pr-04b-conversation-events.spec.md) · [Plan](../spec/pr-04b-conversation-events.plan.md) |
| 4C 执行控制台（`CONSOLE`） | 多项目操作与运行详情；轮次结束自动折叠过程，保留最终回复、未解决问题和阅读位置 | 4B | [Spec](../spec/pr-04-console.spec.md) · [Plan](../spec/pr-04-console.plan.md) |
| 5 人工介入（`INTERVENTION`） | 决策提交、答案应用证明、在线恢复与审计闭环，共用双后端接口 | 4C | [Spec](../spec/pr-05-intervention.spec.md) · [Plan](../spec/pr-05-intervention.plan.md) |

两条链路的精确轮次/最终回复能力需要真实来源证据。公开协议存在缺口时，将协议补齐作为相应功能的前置；执行级展示不能替代精确体验的验证。

### 后续仍依赖的已有基础

- [统一观察契约](../spec/pr-01-observation.spec.md)：供 4B/4C 复用选择、游标、来源换代及原始输出读取规则。
- [服务控制协议](../spec/pr-02-control.spec.md)：供后台生命周期、4A、4C 和人工介入复用请求身份、回执、准入及不确定动作守卫。

## 后续：中心状态机与远程执行

| 步骤 | 目标 | 依赖 | 执行规格 |
| --- | --- | --- | --- |
| 远程协议验证（`REMOTE-PROTOCOL`） | 验证 Orca 真实远端派发、身份、输出、问答、结果及恢复边界，形成可复现证据 | 隔离远端环境和明确的能力门槛 | [Spec 01](../spec/01-orca-remote-protocol.md) |
| 中心执行模型（`EXEC-MODEL`） | 引入 WorkflowRun/Execution/StepAttempt、最小流程状态机、SQLite 事务与旧记录迁移 | 实测协议映射；复用共同后端与会话契约 | [Spec 02](../spec/02-execution-model-and-adapter.md) |
| 分阶段远程闭环（`REMOTE-WORKFLOW`） | 中心逐步派发实现、验证、审查与发布；完成远程观察、人工决策、断线对账及结项 | 远程必需能力验证通过、中心执行模型 | [Spec 03](../spec/03-remote-execution-and-viewer.md) |

远程协议探测可与不依赖其结果的本机工作并行；中心模型的独立设计与本地存储测试不必等待远端环境，但生产远程路径必须满足前置门槛。控制台与中心模型交叉使用同一执行/会话契约，后落地的工作扩展已有权威实现，避免形成两套适配器或业务写者。

## 规划约束

- [架构约束](architecture-constraints.md)
- [控制台约束](console-constraints.md)
