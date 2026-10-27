# 产品设计

本目录维护长期有效的具体产品设计，按职责和功能命名，不按交付阶段或 PR 编号分类。当前执行支持范围包括本地 Codex 与远端 Orca environment；本机 Orca 路径已放弃。已实现契约与拟议设计分开说明，文档本身不证明平台验收通过，也不改变中心固定 Task。

| 设计 | 内容与边界 |
| --- | --- |
| [数据模型](data-model.md) | Task/DAG、执行、反馈、交付、持久化和独立状态；统一格式源码定位 |
| [执行配置](execution-profiles.md) | 项目/全局配置、权限冻结、发布责任和需求准入 |
| [工作区准备](worktree-preparation.md) | 仓库/worktree 身份、准备、锁和失败恢复 |
| [Worker 流程异常处理与重试](worker-lifecycle.md) | 从领取、初始化、输入发送到交付/清理的节点矩阵；统一异常分类、重试预算、会话变化与恢复边界，区分当前实现和待补能力 |
| [可靠性交付链路](../architecture/reliable-delivery-chain.md) | 串联命令、原请求、PR 唯一交付、重启、停服、部署和 reset 的证明；区分必要核验与可简化协调，不另建 Worker 规则 |
| [决策与反馈](decisions.md) | 问题、答案、投递事实、宿主阻断与人工恢复 |
| [会话事件](conversation-events.md) | 来源、规范事件、完整正文、分页、采集与展示 |
| [Activity](activity.md) | 数据库活动摘要、读取游标与服务控制台输出 |
| [已退役的执行控制台](../../SteerlineFlow/archive/design/execution-console.md) | TUI 的历史布局与操作记录；当前使用 Web |
| [Runner 协作契约](../../SteerlineFlow/spec/runner-collaboration.spec.md) | 当前 development/1 与 development-audited/1、执行预设/冻结及跨仓交接；迁出前 [PR 流程](../../SteerlineFlow/spec/worker-pr-flow.spec.md)保留历史追溯 |
| [客户端与 Go 服务边界](planning-architecture.md) | Web/应用/项目 HTTP 边界及历史 #91 追溯 |
| [Web 前端技术方案](web-frontend.md) | 已确认技术栈与首批 DAG/执行页；独立 web 工程及浏览器接入契约见普通开发 Spec/Plan，实际结果归专属验收 |
| [项目管理与统一 Web 入口](project-management.md) | 独立 Web/已有项目切换及统一启动宿主已交付；添加/结构核验/纳管后置，仍有独立 Spec/Plan |
| [项目更新与候选版本维护](project-maintenance.md) | 已实施：远端/本地提交与快进、自动材料/测试生成、候选独立冻结及过期保留；旧PR仍可完成任务，固定构建验证及未验边界见专属验收 |
| [添加项目准备提示词（方案）](project-onboarding-prompt.md) | 用户复制到目标仓库 Codex 执行的文档准备模板；不创建项目状态或 Task |
| [项目结构与纳管核验（方案）](project-structure.md) | 文档角色/路径清单、只读固定版本核验、纳管与执行资格边界；尚无核验器 |
| [本机客户端协议 v1](local-client-protocol.md) | 现有回环协议与公共查询/命令接口 |
| [规划界面原型参考](../../SteerlineFlow/archive/design/planning-ui.md) | 早期 Web 规划方案；部分构想未交付，不产生当前任务/前置 |
| [Roadmap 读取与转正](roadmap-promotion.md) | 只读 Markdown、Runner 解析、内部代号确认、文档能力前置与 Task 事务内入图编号 |
| [规划界面原型](prototypes/planning-ui/README.md) | 单文件交互演示；不连接真实服务、不代表已实现产品 |

Web 是当前唯一交互前端；旧任务 #91 交付 HTTP 客户端基线，之后 TUI 已按[退役 Spec](../../SteerlineFlow/spec/tui-retirement.spec.md)删除。早期 TUI/规划界面文档已移至[设计归档](../../SteerlineFlow/archive/design/README.md)，实现与验收状态以当前 Spec 和专属验收为准。

整体职责见[架构总览](../architecture.md)，代码与接口归属见[源码索引](../architecture/current-code-architecture.md)，执行要求见[Spec 索引](../../SteerlineFlow/spec/README.md)，测试入口见[测试指南](../../SteerlineFlow/testing.md)。历史配置、界面方案与试验只在[归档](../../SteerlineFlow/archive/README.md)或[验证证据](../../SteerlineFlow/testing/validation)中承接。

## 维护规则

每条规则由一个设计文档负责，其他文档链接它。数据模型维护共同身份与事务边界；配置、反馈和会话细节分别归各自文档，不反复复制。格式版本以源码为准，统一表放在数据模型；历史报告中的版本不改写成当前版本。

设计文档解释产品行为和约束，不积累 PR 变更日志或验收过程。实现与证据有差距时写明范围和限制，剩余任务留在 roadmap/Spec。被替代的规则直接删除；独有历史材料归档，重复材料无需另存。目录调整只修复导航，不改固定 commit 中的契约路径/摘要，也不改写原始证据。
