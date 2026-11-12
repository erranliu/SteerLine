# 文档入口

SteerLine 的整体职责、具体产品设计、后续规划、执行契约与验证证据分别维护。当前通过独立 SteerRunner 接入本地 Codex、Windows/amd64 本机 Orca 和远端 Orca environment；具体组合及执行资格见[执行方案](product-design/execution-configuration.md)。实际验证按专属验收中的固定构建与范围判断；规划与原型的拟议设计不表示已经交付。

| 阅读目的 | 入口 |
| --- | --- |
| SteerLine 的产品介绍和工作方式 | [SteerLine](../README.md) |
| 项目流程设计、用户参与及 Runner 职责 | [项目流程与 Runner 职责](product-design/project-workflow.md) |
| 首次准备项目测试材料、环境接口并接入 | [项目接入指南](../SteerlineProjectGuideDoc/README.md) |
| 本项目的规划、任务与验证流程 | [SteerlineFlow](../SteerlineFlow/README.md) |
| 系统整体职责与业务权威 | [架构总览](architecture.md) |
| Runner 依赖、能力要求及当前升级边界 | [Runner 接入](integration/steerrunner.md) |
| 安装更新、同构建重启与换版排空 | [安装部署](integration/install-deploy.md) |
| 隔离调试包、重置与真实 PR 调试 | [独立调试](runtime-install-debug.md) |
| 本机应用 API、命令回执与重连 | [本机客户端协议](product-design/local-client-protocol.md) |
| 主工作区同步、显式准备与新候选就绪 | [项目维护](product-design/project-maintenance.md) |
| SteerAgent 多轮重构、预算、PR 与冲突收益审计（产品设计） | [重构与维护](product-design/refactoring-maintenance.md) |
| 重构与维护的实现边界与 Runner 接入前提 | [维护实现方案](architecture/refactoring-maintenance.md) |
| 当前进程、模块、客户端与事务边界 | [代码架构](architecture/current-code-architecture.md) |
| Web 技术栈、构建启动与工程使用 | [Web README](../web/README.md) |
| 执行预设、配置生效、开发覆盖与发布限制 | [执行配置](product-design/execution-configuration.md) |
| 活动、最近消息与完整会话结果 | [执行观察与结果](product-design/execution-observability.md) |
| 当前项目切换与拟议添加/核验 | [项目接入与管理](product-design/project-onboarding.md) |
| 固定提示词变更、独立审核及保护边界 | [提示词治理](prompt-governance.md) |
| 规划任务前判断并行与冲突 | [并行开发指南](architecture/parallel-development.md) |
| 配置、数据、反馈、会话与 UI 设计 | [产品设计](product-design/README.md) |
| Runner 调用生命周期、异常与恢复 | [Runner 调用生命周期](product-design/runner-lifecycle.md) |
| 命令受理、唯一交付与资源结算的核验关系 | [可靠性交付链路](architecture/reliable-delivery-chain.md) |
| 讨论目标与执行任务 | [RoadmapGuide](../SteerlineFlow/Roadmap/RoadmapGuide.md) · [正式执行规划](../SteerlineFlow/Roadmap/execution-map.md) · [产品愿景](../SteerlineFlow/Roadmap/product-vision.md) |
| 未交付规格与待补验收 | [Spec / Plan](../SteerlineFlow/spec/README.md) |
| 全项目测试方法与入口 | [测试指南](../SteerlineFlow/testing.md) · [分级 E2E](../tests/e2e/README.md) |
| 历史规格、流程与配置 | [归档](../SteerlineFlow/archive/README.md) |
| 自动/人工验收流程、已验与待验 | [acceptance/](../SteerlineFlow/testing/acceptance/README.md) |

## 目录边界

- 仓库根目录的 `SteerlineProjectGuideDoc/` 维护 SteerLine 提供的固定通用规范；`SteerlineFlow/` 维护本项目的开发流程材料；`docs/` 维护产品设计、架构和接入说明。
- docs 根目录维护总入口、架构总览、提示词治理和独立调试指南；测试总入口位于 `SteerlineFlow/testing.md`。
- product-design/ 放长期维护的具体产品设计；按功能命名，明确当前契约、拟议方案和验证限制，不按阶段编号积累目录。
- architecture/ 放当前代码架构、源码索引与持续维护的并行开发指南；指南链接契约和测试，缺少独立说明时记录当前边界，不重复维护产品规则。
- integration/ 放 Runner 接入、安装部署和测试进度通道；执行预设与开发开关统一在产品设计的执行配置中维护。
- roadmap、Spec、Plan、测试清单、验收计划与历史报告、过期流程文档的维护入口统一见 [SteerlineFlow](../SteerlineFlow/README.md)。产品设计通过链接引用这些材料，不替代任务固定契约。

新设计进入对应子目录并更新索引；整体文档只保留摘要和链接。规则有唯一维护位置，持久格式以[源码定位表](product-design/data-model.md#当前格式来源)为准。移动时修复当前导航，保留历史 Canonical Spec 路径和固定契约；不让导航调整改写中心 Task 或外部资源状态。

文档调整用 scripts/check_doc_links.py、roadmap_task_id.py check 和 git diff --check 验证；纯导航、文字及源码注释修改不启动产品或模型测试。
