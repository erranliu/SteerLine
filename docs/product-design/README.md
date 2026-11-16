# 产品设计

本目录按领域维护长期规则。下列当前能力不等于每个平台或安装版均已验收，实际范围见各页专属证据；拟议接入能力单独列出。整体职责见[架构总览](../architecture.md)，源码归属见[代码架构](../architecture/current-code-architecture.md)。

## 当前产品规则

| 设计 | 维护范围 |
| --- | --- |
| [项目流程与 Runner 职责](project-workflow.md) | 开发闭环、用户参与和职责；当前能力与未来总体设计 |
| [人工核验](manual-verification.md) | Plan 伴随文件、草稿发布、PR 核验来源、现场和待办关闭 |
| [SteerAgent 能力与协作边界](steer-agent.md) | 材料生成、对话续作、Roadmap 规划、持续维护的长期分类、解耦与 Runner 契约；当前能力及缺口分别说明 |
| [项目接入与管理](project-onboarding.md) | 当前项目切换与隔离；拟议添加、结构核验和登记单独成章 |
| [项目维护](project-maintenance.md) | 显式材料准备、统一就绪、候选冻结与过期保留 |
| [数据模型](data-model.md) | 实体、身份、事务不变量与格式源码定位 |
| [执行配置](execution-configuration.md) | 预设、条件保存、生效与冻结、两层默认链路及开发覆盖/发布限制 |
| [Runner 调用生命周期](runner-lifecycle.md) | 调用方资格、原请求、反馈、交付和停止；W/E/V 定位 |
| [可靠性交付链路](../architecture/reliable-delivery-chain.md) | 命令受理、唯一交付与资源结算的核验关系 |
| [问题、反馈与恢复输入](decisions.md) | 公开问题/controls 的投影、校验及送达事实 |
| [执行观察与结果](execution-observability.md) | 活动/游标、最近消息、verified final、原正文与分页；各自事实含义分开 |
| [异步活动中心](async-activities.md) | 检测、测试准备和候选的统一只读观察；业务操作仍由原 owner 办理 |
| [运行时测试进度](../integration/runtime-progress.md) | 固定字段、正式 writer 协议和原 Run 快照 |
| [本机客户端协议](local-client-protocol.md) | 应用 v2 路由、命令、回执与重连 |
| [Roadmap 读取与转正](roadmap-promotion.md) | 静态准备、解析/文档记录、发布及 Task 事务入图 |

## 工程与维护入口

| 入口 | 职责 |
| --- | --- |
| [客户端与服务边界](../architecture/current-code-architecture.md#客户端与服务边界) | Web、应用 API、项目端口及状态所有权 |
| [重构与维护实现方案](../architecture/refactoring-maintenance.md) | 维护业务、只读决策与 Runner 执行的边界，以及接入前提 |
| [Web 工程使用](../../web/README.md) | 技术栈、认证、构建启动、缓存与隔离验证 |
| [Runner 协作契约](../../SteerlineFlow/spec/runner-collaboration.spec.md) | 所选构建公开 workflow、执行预设及跨仓交接 |
| [提示词治理](../prompt-governance.md) | 固定正文、来源、实际差异和独立审核 |
| [Spec 索引](../../SteerlineFlow/spec/README.md) · [测试指南](../../SteerlineFlow/testing.md) | 单次变更要求、验证与证据 |

计费与采集设置由[计费职责 Spec](../../SteerlineFlow/spec/billing-ownership.spec.md)及[验收](../../SteerlineFlow/testing/acceptance/billing-ownership.md)维护；不在本索引复制规则或部署状态。

## 拟议能力与项目接入

| 方案 | 当前限制 |
| --- | --- |
| [重构与维护](refactoring-maintenance.md) | 选择已就绪运行环境，每项目默认并发三条行动，超额排队。默认谨慎与 1 轮，预算耗尽自动收尾，支持 PR 报告及手动冲突续作 |
| [对话续作：给定上下文提示词建议](main-agent.md) | 原“主 Agent”的现有建议能力；明确待答自动接入 Runner generate，本地只读生成、活动与查看/复制；未部署 |
| [添加、核验与登记](project-onboarding.md#拟议添加流程) | 角色清单及 PS-01–PS-07；完整 UI、核验器和正式登记尚未实现 |
| [项目准备提示词](project-onboarding-prompt.md) | 用户自行复制使用；不触发产品 Agent、纳管或 Task |

现有配置与切换不表示结构已通过；当前准备方法以[项目初始化指南](../../SteerlineProjectGuideDoc/project-initialization.md)为准。

## 历史与维护规则

当前唯一交互前端为 Web；TUI 已按[退役 Spec](../../SteerlineFlow/spec/tui-retirement.spec.md)删除。[旧执行控制台](../../SteerlineFlow/archive/design/execution-console.md)、迁出前[PR 流程](../../SteerlineFlow/spec/worker-pr-flow.spec.md)、早期[规划原型](../../SteerlineFlow/archive/design/prototypes/planning-ui/README.md)归[设计归档](../../SteerlineFlow/archive/design/README.md)。其他历史配置、试验和报告见[归档](../../SteerlineFlow/archive/README.md)与[验证证据](../../SteerlineFlow/testing/validation)。

每条规则保留一个正文 owner，其他页链接引用。数据模型维护身份、事务和格式定位，功能页维护领域规则；Spec/Plan 固定单次任务，验收记录证明实际结果。索引只维护导航与状态摘要，不积累实现日志。

被替代的独有材料保留历史入口，重复段不另存；调整导航不改固定 commit 的路径/摘要或原始证据。拟议设计与未验范围按实际交付更新，不因文档存在而宣称能力已实现。
