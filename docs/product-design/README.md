# 产品设计

本目录维护长期有效的具体产品设计，按职责和功能命名，不按交付阶段或 PR 编号分类。当前执行支持范围是本地 Codex 与本机 Orca。已实现契约与拟议设计分开说明，文档本身不证明平台验收通过，也不改变中心固定 Task。

| 设计 | 内容与边界 |
| --- | --- |
| [数据模型](data-model.md) | Task/DAG、执行、反馈、交付、持久化和独立状态；统一格式源码定位 |
| [执行配置](execution-profiles.md) | 项目/全局配置、权限冻结、发布责任和需求准入 |
| [工作区准备](worktree-preparation.md) | 仓库/worktree 身份、准备、锁和失败恢复 |
| [决策与反馈](decisions.md) | 问题、答案、投递事实、宿主阻断与人工恢复 |
| [会话事件](conversation-events.md) | 来源、规范事件、完整正文、分页、采集与展示 |
| [Activity](activity.md) | 数据库活动摘要、读取游标与服务控制台输出 |
| [规划与原生客户端架构](planning-architecture.md) | 拟议的 Go 服务、WinUI 客户端及 Codex 规划会话边界 |
| [规划与原生界面交互](planning-ui.md) | 拟议的规划对话、草稿审阅、DAG、执行与待处理页面 |
| [规划界面原型](prototypes/planning-ui/README.md) | 单文件交互演示；不连接真实服务、不代表已实现产品 |

规划与 UI 的新增方向是自由草稿、独立审核和受控正式准入，见[目标](../roadmap/phase2-draft-roadmap-admission.md)。现有 Publication/ID 预留细节仍是待对齐的基础方案，不宣称已实现审核协议。选定实施前与独立 Spec/Plan 一并固定；推进顺序见[二期交付边界](../roadmap/phase2-delivery.md)。

整体职责见[架构总览](../architecture.md)，代码与接口归属见[源码索引](../architecture/current-code-architecture.md)，执行要求见[Spec 索引](../spec/README.md)，测试入口见[测试指南](../testing.md)。历史配置与试验只在[归档](../archive/README.md)或[验证证据](../validation/)中承接。

## 维护规则

每条规则由一个设计文档负责，其他文档链接它。数据模型维护共同身份与事务边界；配置、反馈和会话细节分别归各自文档，不反复复制。格式版本以源码为准，统一表放在数据模型；历史报告中的版本不改写成当前版本。

设计文档解释产品行为和约束，不积累 PR 变更日志或验收过程。实现与证据有差距时写明范围和限制，剩余任务留在 roadmap/Spec。被替代的规则直接删除；独有历史材料归档，重复材料无需另存。目录调整只修复导航，不改固定 commit 中的契约路径/摘要，也不改写原始证据。
