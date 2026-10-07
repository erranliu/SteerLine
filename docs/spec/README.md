# 当前 Spec 与 Plan

本目录保留尚未交付的执行规格、未闭合的定向验收和仍适用的共同约束。第一阶段只运行和验收本地 Codex 与本机 Orca。每个活动任务对应一份独立 Spec、Plan 和实现 PR。

已执行的 Spec 与配套 Plan 见[已交付归档](../archive/spec/completed/README.md)；退役流程和远端试验见[历史资料](../archive/spec/retired/README.md)。归档中的旧状态文字和路径属于原交付时点，不能当作当前待办。任务 ID 身份记录与原始验证数据保留。

## 产品精简（三批，待实施）

目标：`PRODUCT-SIMPLIFICATION`。依据 2026-10-07 用户接受的取舍及源码审计，完整保留多后端、远端执行路径、多候选、原会话反馈与唯一 PR 交付。下列新规格仅定义三批产品收敛，不改变当前一期验收范围，不宣称远端权限证据已补齐；未分配 Task ID；第一批已按三窗修订实现/验证，PR #231待评审并有实施Plan，第二/三批仍待实施。

每批后续一份实施 Plan、一个独立实现 PR，按顺序交付。共同边界及与历史规格的覆盖关系见第一批和各批正文。

| 批次 | 范围 | 硬前置 | Spec |
| --- | --- | --- | --- |
| 1 | 三窗概要/最近10条活动/右下内嵌反馈、退役过程阅读、只持久草稿及请求 | 无本系列未完成前置 | [精简候选控制台与会话阅读](product-simplification-01-console.spec.md) · [Plan](product-simplification-01-console.plan.md) |
| 2 | 两种明确权限模式、移除附加执行需求与认证入口 | 第一批合入 | [收窄执行权限配置与移除附加需求](product-simplification-02-configuration.spec.md) |
| 3 | Task 丢弃后资源人工处置、重复 CLI 与高级图查询/导出退役 | 第二批合入 | [保留资源人工处置与精简命令入口](product-simplification-03-resources-cli.spec.md) |

## 控制台体验优化

目标：`CONSOLE-PRIMARY-WORKSPACE`。依据 2026-10-07 用户确认的三窗主界面操作与可读性方案；未分配 Task ID，实施及验证中，已有[实施 Plan](console-primary-workspace.plan.md)。以产品精简第一批的三窗实现为功能前置，不依赖第二/三批；后续一份实施 Plan、一个独立实现 PR。

| 范围 | Spec |
| --- | --- |
| 主界面方案选择/确认/回答/核验与结果摘要、收窄详情入口、颜色/加粗和明确焦点 | [三窗主界面操作与控制台可读性优化](console-primary-workspace.spec.md) |

## 当前执行规格

| 任务 ID | 任务名称 | 硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | [Spec](local-runtime-isolation.spec.md) | [Plan](local-runtime-isolation.plan.md) |
| 42 | 第一阶段集成验收 | `#39`、`#83` | [Spec](phase1-release-acceptance.spec.md) | [Plan](phase1-release-acceptance.plan.md) |
| 43 | Codex 规划主 agent 能力验证 | 无 | [Spec](phase2-agent-capabilities.spec.md) | [Plan](phase2-agent-capabilities.plan.md) |
| 44 | WinUI 基座与原生输入 | 无 | [Spec](phase2-winui-foundation.spec.md) | [Plan](phase2-winui-foundation.plan.md) |
| 45 | 本机客户端协议与历史查询 | `#36` | [Spec](phase2-local-api.spec.md) | [Plan](phase2-local-api.plan.md) |
| 46 | 规划提案与受控草稿 | `#45` | [Spec](phase2-planning-drafts.spec.md) | [Plan](phase2-planning-drafts.plan.md) |
| 47 | Codex 规划会话服务 | `#43`、`#46` | [Spec](phase2-planning-sessions.spec.md) | [Plan](phase2-planning-sessions.plan.md) |
| 48 | 正式文档发布与任务创建 | `#46` | [Spec](phase2-document-publish.spec.md) | [Plan](phase2-document-publish.plan.md) |
| 49 | Roadmap 与 DAG 原生浏览 | `#44`、`#48` | [Spec](phase2-roadmap-ui.spec.md) | [Plan](phase2-roadmap-ui.plan.md) |
| 50 | 执行与待处理原生界面 | `#44`、`#45` | [Spec](phase2-execution-ui.spec.md) | [Plan](phase2-execution-ui.plan.md) |
| 51 | 规划对话原生界面 | `#44`、`#47` | [Spec](phase2-conversation-ui.spec.md) | [Plan](phase2-conversation-ui.plan.md) |
| 52 | 文档审阅与确认原生界面 | `#44`、`#48` | [Spec](phase2-review-ui.spec.md) | [Plan](phase2-review-ui.plan.md) |
| 53 | 第二阶段完整闭环验收 | `#49`、`#50`、`#51`、`#52`、`#42` | [Spec](phase2-integration.spec.md) | [Plan](phase2-integration.plan.md) |

## 延后版本草案

下列任务未交付，但不作为当前执行入口或一期/二期前置；重新启用前须按实际维护需求、当前格式和 #83 已具备能力重定 Spec/Plan 与测试计划。保留身份不代表中心已暂停或丢弃对应 Task。

| 任务 ID | 任务名称 | 原草案硬前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 40 | 固定版本与双入口 | 无 | [Spec](versioned-runtime.spec.md) | [Plan](versioned-runtime.plan.md) |
| 41 | dev 隔离与 stable 提升 | `#40`、`#36` | [Spec](runtime-isolation-promotion.spec.md) | [Plan](runtime-isolation-promotion.plan.md) |

## 尚未闭合的定向验收

下列实现已有合入基线，但仍有明确的验证缺口；不因归档其他已交付任务而宣告完成，也不重新创建任务身份。

| 范围 | Spec / Plan | 剩余边界与证据 |
| --- | --- | --- |
| local Codex app-server | [Spec](local-codex-app-server.spec.md) · [Plan](local-codex-app-server.plan.md) | A/B/C 已完成，定向 race detector 仍受工具链条件限制；见[状态](../validation/local-codex-app-server/status.md) |
| 本机 Orca 原生编排 | [Spec](worker-flow-orca-native.spec.md) · [Plan](worker-flow-orca-native.plan.md) | 活动轮次输入、消息竞争/故障、完整输出和独立生命周期仍按原 ON 场景核验；见[实现审计](../validation/worker-flow/implementation-audit.md) |
| worker 停服恢复 | [Spec](worker-service-recovery.spec.md) · [Plan](worker-service-recovery.plan.md) | local 闭环已验证，Orca 新 agent 安全重建仍缺真机证据；见[验证记录](../validation/worker-service-recovery/README.md) |

## 共同约束

- [执行架构](execution-architecture.spec.md)：当前持续 worker、单写者、身份、反馈与真实交付边界。
- [原生任务](native-task-constraints.md)：固定契约、DAG、唯一权威和受控命令。
- [第一阶段](phase1-delivery-constraints.md)：本机候选、安装/调试隔离与单安装版组合验收交接。
- [第二阶段](phase2-constraints.md)：受控规划、文档发布、原生 UI 和公共传输。

当前实现与模块边界见[代码架构](../architecture/current-code-architecture.md)，测试选择见[测试指南](../testing.md)。已完成的执行隔离、主包解耦与三线迁移规格及其阶段约束一并归档，不再列作活动执行入口。
