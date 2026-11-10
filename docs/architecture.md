# 整体架构与职责边界

SteerLine 管理项目任务、依赖、候选及交付，通过独立 SteerRunner 执行开发和辅助生成。本文维护系统组件与业务权威；项目工作方式及当前能力、未来设计见[项目流程](product-design/project-workflow.md)，实现位置见[代码架构](architecture/current-code-architecture.md)。

## 职责与权威

| 组件 | 负责内容 | 边界 |
| --- | --- | --- |
| Web 与业务 CLI | 用户交互、查询产品状态、提交原目标与版本绑定的命令 | 客户端缓存、草稿和原信封不成为业务状态，不直接访问任务库或原生执行资源 |
| 应用目录与项目运行时 | 配置冻结、项目装载、资源组装、实例与锁、项目产品端口 | 每项目状态隔离，共享宿主及 home 级 Runner；项目运行时唯一关闭其本地资源 |
| 任务应用与调用方协调 | Task/DAG、候选资格、原请求、公开观察、用户反馈、唯一交付和停止义务 | 不管理 Runner 内部步骤或解析模型正文推进执行 |
| 主 Agent 契约与核心 | 明确上下文的校验、固定输入组装和单一提示词结果校验 | 不读取项目或调用工具，不持有业务状态、Runner 或存储；采用及执行在模块外 |
| 主 Agent 宿主适配 | 来源读取、冻结输入、异步记录、取消与活动投影 | 经 Runner generate/1 本地只读生成；接受其原生工具边界，不承担业务采用或执行 |
| Runner | 每个 Run 内的工作区、模型与原生 Agent、会话、执行工作流、逐轮输入和资源结算 | 每个 Run 只承担一个明确任务；只通过公开契约协作，不接管任务间协调，不取得 SteerLine 任务库写权 |
| PR 平台 | PR 身份、目标及实际合并事实 | 本次交付使用实时核验结果，缓存或模型总结不代替平台事实 |
| SQLite 与产品查询层 | 存储层核验并原子保存业务事实；查询层生成产品投影 | 存储不构造产品 DTO，投影不产生新的业务权威 |

用户决定目标、关键取舍与授权范围，执行 Agent 完成具体工作。Task、候选、Runner Run、原生会话、PR 和资源停止分别表达不同事实；配置、checkpoint、日志及 UI 状态不替代它们。

## LLM 执行边界

全部 LLM 执行统一经 Runner，包括开发任务和产品功能的辅助生成。SteerLine 提供业务目标、材料与授权并消费结果，不自行接入模型或启动原生 Agent。Runner 维护工作流、审计、修复及补报，调用方不另建执行状态机。

Runner 在任务要求与授权内管理内部 workflow；需要补充任务依据、改变任务范围或协调其他任务时，保留现场并上报，由 SteerLine 组织解决及授权后续执行。职责原则、任务内问题的判断和当前未完成的补材料闭环见[项目流程的职责边界](product-design/project-workflow.md#4-steerline-与-runner-的职责边界)。

原生目录、模型和配置语义由固定 Runner EXE 的无服务接口提供；Web 负责逻辑编辑、产品支持判断和条件保存。目录查询不创建业务模型轮次，也不授予项目执行资格。接入要求见[Runner 接入](integration/steerrunner.md)，SteerLine 预设选择与 Runner 内部默认链路分别由[执行配置](product-design/execution-configuration.md#两层默认链路)说明。

## 执行与交付

外部动作前保存固定意图、材料和原请求；未知只对账原操作，不换身份重做。Runner 公开结果供调用方观察，任务交付由 SteerLine 直接向平台核验原候选 PR，再原子记录唯一交付、关闭 Task、释放后继和保存全部候选停止义务。

执行结束、PR 就绪和资源停止均不独立证明任务交付；任一合格候选可交付，不受兄弟查询未知阻塞。完整事务与核验关系唯一维护在[可靠性交付链路](architecture/reliable-delivery-chain.md)，身份及格式见[数据模型](product-design/data-model.md)。

## 生命周期、反馈与恢复

共享后端承载独立的项目运行时，应用宿主持有一个 home 级 Runner。关闭 Web 只结束观察；项目本地关闭、共享服务退出和原生资源结算分别核验。反馈和恢复绑定原目标、当前资格与授权；当前 Runner v8 换构建须先排空，不接管活动任务。节点、异常、候选退出及停止规则见[Worker 生命周期](product-design/worker-lifecycle.md)，操作见[安装部署](integration/install-deploy.md)。

## 文档职责

架构服务于可独立理解、实现、验证和交付的功能边界。并行判断见[并行开发指南](architecture/parallel-development.md)，产品规则归[具体设计](product-design/README.md)，工程使用归[Web README](../web/README.md)。普通开发由当前聊天实施；仅用户明确要求时按 roadmap 调度，入口见[项目规则](../AGENTS.md)。

本文保持当前组件摘要；历轮解耦、客户端退役、审计及未实施建议从[历史入口](../SteerlineFlow/archive/design/architecture-web-history.md)追溯。不兼容未发布旧格式，不新增迁移、双写或回退；本轮文档整理不改变固定任务契约、实现或验证范围。
