# SteerLine

**SteerLine 是一个 AI Workshop，用多 Agent 协作把软件项目的规划、并行开发和交付核验串成一条可追踪的流程。** 你可以在 Web 中和 SteerAgent 讨论一项需求或整份 Roadmap，审阅任务范围、依赖和交付要求，再通过 SteerRunner 组织执行 Agent 开发。SteerLine 核验 PR 的实际合并结果，并据此判断哪些后续任务具备开工条件。

项目尚未发布，目前可在 Windows 上从源码部署体验。 [安装部署](docs/integration/install-deploy.md) · [项目接入指南](SteerlineProjectGuideDoc/README.md)

## 一次开发怎么推进

以「让用户下载报表」为例，可以拆成「提供导出接口」和「在页面接入下载」两项任务。

1. **规划与确认。** 和 SteerAgent 讨论导出格式、权限和失败场景，确认页面任务依赖接口交付。Spec 写清成果和通过条件，Plan 安排实现与验证，验收文档记录核验方式。
2. **发布文档并入图。** 审阅生成的任务文档，发布并合并文档 PR。SteerLine 核实合并后，可将任务加入 DAG（有向无环依赖图）。满足各自前置和环境要求的独立任务可以并行。
3. **执行与核验。** SteerRunner 按固定的任务材料和执行方案组织实现、测试和 PR 发布。如果 Plan 要求人工核验，核验人可以从 Web 启动候选版本并记录结果。
4. **交付与接续。** 接口 PR 合并后，SteerLine 核验实际交付。页面任务的前置条件随之满足，能否开工还取决于运行环境等条件，是否自动启动由项目的派发设置决定。

[Roadmap 读取与转正](docs/product-design/roadmap-promotion.md) · [任务启动与候选资格验收](SteerlineFlow/testing/acceptance/task-start-eligibility.md)

## 当前能力

| 能力 | 在 Web 中可以做什么 |
| --- | --- |
| 项目接入与环境检测 | 添加已初始化的 Git 项目，核验目标分支材料并保存配置，重启后装载。检查各运行环境的就绪状态和原因。[项目接入](docs/product-design/project-onboarding.md) |
| Roadmap 与单任务规划 | 多轮讨论并修改 Roadmap 草稿，或先沟通一项需求，再在原会话生成和更新任务卡。审阅依赖、生成任务文档，经文档 PR 合并核验后入图。[规划验收](SteerlineFlow/testing/acceptance/roadmap-planning-workspace.md) · [任务入图](docs/product-design/roadmap-promotion.md) |
| 任务执行与交付 | 查看任务 DAG、执行进展和会话，在原执行中处理待答问题，并跟踪 PR 合并及后续任务的开工资格。[执行观察](docs/product-design/execution-observability.md) · [执行配置](docs/product-design/execution-configuration.md) |
| 人工核验 | 对 Plan 指定的核验项，从草稿 PR 对应的提交启动候选成果，记录结论。负责人在 PR 平台决定何时标为 Ready 和合并。[人工核验](docs/product-design/manual-verification.md) |
| 重构与维护 | 选择范围、运行环境和预算，让 SteerAgent 分轮组织重构与验证，由 Runner 交付 PR 和报告。[维护设计](docs/product-design/refactoring-maintenance.md) · [维护验收](SteerlineFlow/testing/acceptance/refactoring-maintenance-concurrency.md) |

开发任务通过独立的 SteerRunner 接入本机 Codex、本机 Orca 和远端 Orca。可用组合取决于 Runner 构建和运行环境，实际验证范围见各功能的专属记录。由 SteerAgent 在授权范围内贯穿完整项目周期的自主推进仍是产品方向。 [Runner 接入](docs/integration/steerrunner.md) · [验收记录](SteerlineFlow/testing/acceptance/README.md)

## 开始体验

- **在 Windows 上从源码部署。** 准备 Git、Go、Node.js/npm 和匹配的 SteerRunner 源码。首次部署可按指南指定 Runner 源码路径，构建并启动服务。Web 默认地址为 `http://127.0.0.1:18742`。 [安装部署](docs/integration/install-deploy.md) · [Web 构建与调试](web/README.md)
- **接入自己的 Git 项目。** 按指南准备项目约定、测试入口和环境检测脚本，提交到目标分支并推送。然后在 Web 的项目切换器或零项目页面添加项目，核验并保存配置，重启后检查执行方案和运行环境。 [项目接入指南](SteerlineProjectGuideDoc/README.md)

## 设计方法与愿景

- **让并行来自任务边界。** 用户决定目标、优先级和关键取舍。SteerLine 维护项目任务与依赖，SteerRunner 管理具体执行，执行 Agent 按固定材料完成任务。 [项目流程与 Runner 职责](docs/product-design/project-workflow.md)
- **让文档成为工作依据。** 项目约定写明共同要求，Roadmap 管理整体推进，Spec 和 Plan 固定单项任务的交付与实施依据，验收记录保存实际结果。 [受管理项目规范](SteerlineProjectGuideDoc/project-standards.md)
- **在规划时就定义怎样算交付。** 把通过条件和验证方式写进任务材料，在执行后按实际提交、测试结果和人工核验记录判断交付。 [测试指南](SteerlineFlow/testing.md)

SteerLine 自身也采用这套方法。 [本仓库开发流程](SteerlineFlow/README.md) · [文档入口](docs/README.md) · [产品愿景](SteerlineFlow/Roadmap/product-vision.md)
