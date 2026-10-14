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
| [执行控制台](execution-console.md) | 当前 TUI 的任务/候选布局、原地反馈、能力展示与操作速查 |
| [Worker 步骤流程 Spec](../spec/worker-staged-flow.spec.md) | #84 已纳入一期收尾执行规划：现有 worker、简单步骤序列和 adapter；状态判断、三个短提示词及源码改造范围，implement 两轮/publish 单轮；尚未实现 |
| [客户端与 Go 服务边界](planning-architecture.md) | 当前 #91 的 TUI/HTTP 源码边界和未来范围 |
| [Web 前端技术方案](web-frontend.md) | 已确认技术栈与首批 DAG/执行页；独立 web 工程及浏览器接入契约见普通开发 Spec/Plan，实际结果归专属验收 |
| [本机客户端协议 v1](local-client-protocol.md) | #45 回环协议与 #91 TUI 所需接口 |
| [规划界面原型参考](planning-ui.md) | 旧页面方向仅参考，不产生当前任务/前置 |
| [规划界面原型](prototypes/planning-ui/README.md) | 单文件交互演示；不连接真实服务、不代表已实现产品 |

2026-10-13 用户确认当前规划仅到[任务 #91](../spec/tui-http-client.spec.md)。旧三节点/候选已清理，编号不复用；原型与退役规格不作为前置或派发入口。当前 #45 协议已交付，TUI 已改接 HTTP，#44 工程已删除，验收状态见专属记录，见[推进边界](../roadmap/drafts/phase2-delivery.md)。

整体职责见[架构总览](../architecture.md)，代码与接口归属见[源码索引](../architecture/current-code-architecture.md)，执行要求见[Spec 索引](../spec/README.md)，测试入口见[测试指南](../testing.md)。历史配置与试验只在[归档](../archive/README.md)或[验证证据](../validation)中承接。

## 维护规则

每条规则由一个设计文档负责，其他文档链接它。数据模型维护共同身份与事务边界；配置、反馈和会话细节分别归各自文档，不反复复制。格式版本以源码为准，统一表放在数据模型；历史报告中的版本不改写成当前版本。

设计文档解释产品行为和约束，不积累 PR 变更日志或验收过程。实现与证据有差距时写明范围和限制，剩余任务留在 roadmap/Spec。被替代的规则直接删除；独有历史材料归档，重复材料无需另存。目录调整只修复导航，不改固定 commit 中的契约路径/摘要，也不改写原始证据。
