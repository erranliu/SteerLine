# 当前 Spec 与 Plan

本目录保留独立Spec/Plan、未闭合的定向验收和仍适用的共同约束。第一阶段包含本地 Codex、本机 Orca 与远端路径交付。#83/#84/#45/#44已按[正式执行规划](../roadmap/execution-map.md)实现并合入，历史规格和交付记录保留；一期签收及 #84/#44 的人工项仍待验。后续候选材料见[草稿](../roadmap/drafts/roadmap.md)，不重新派发已交付任务。日常派发核对用户确认范围、状态、依赖和固定文档版本，草稿或编号不作为执行来源。

Spec 中的验收表定义要求，Plan 定义实施及最小测试安排；具体自动/人工步骤与实时已验/待验结果统一维护于[acceptance](../acceptance/README.md)。草稿阶段就建立任务专属的一份或多份验收文档并链接，单份文档不覆盖多任务；Spec/Plan 后续细化并绑定场景 ID。execution-map 在纳入执行或实际验收安排后同步验收汇总及未验文档。旧计划中的 docs/validation 路径只用于定位原证据，不作为新结果写入入口：新原始日志用 runner 默认临时目录或本次新建的空临时目录，长期记录只提交 acceptance 的最小摘要。人工为主活动不作为执行任务；少量人工项按允许范围后置。

已执行的 Spec 与配套 Plan 见[已交付归档](../archive/spec/completed/README.md)；退役流程和远端试验见[历史资料](../archive/spec/retired/README.md)。归档中的旧状态文字和路径属于原交付时点，不能当作当前待办。任务 ID 身份记录保留；原始验证数据从[清理前固定 Git 快照](../acceptance/archive/completed/history.md#历史取证)读取。

## 已交付产品精简与控制台

三批产品精简已分别通过PR #231/#236/#238合入，控制台体验增量通过PR #234合入；验收仍绑定各自原构建/范围，不据此宣称当前重跑。这里保留导航，不进入近期执行任务。

| 范围 | 规格 / 计划 | 历史验收 |
| --- | --- | --- |
| 三窗/近期活动/内嵌反馈 | [Spec](product-simplification-01-console.spec.md) · [Plan](product-simplification-01-console.plan.md) | [记录](../acceptance/archive/completed/product-console.md) |
| 固定原生权限与审核配置、附加需求/认证收口 | [Spec](product-simplification-02-configuration.spec.md) · [Plan](product-simplification-02-configuration.plan.md) | [记录](../acceptance/archive/completed/product-simplification-02-configuration/README.md) |
| Task丢弃后资源人工处置、CLI及高级图收口 | [Spec](product-simplification-03-resources-cli.spec.md) · [Plan](product-simplification-03-resources-cli.plan.md) | [记录](../acceptance/archive/completed/product-simplification-03-resources-cli/README.md) |
| 控制台主界面与可读性 | [Spec](console-primary-workspace.spec.md) · [Plan](console-primary-workspace.plan.md) | [记录](../acceptance/archive/completed/console-primary-workspace.md) |

任务Plan逐项或按明确ID范围覆盖全部自动验收项，列场景/命令、环境、时点、通过条件与专属登记位置；缺驱动/场景/环境能力须在实施中补齐或明确前置。必要回归与产品自动验收分别标记，静态检查不代替业务/原生证据。

## 旧 Orca 编排身份残留清理（普通开发规格）

删除当前后端不再使用的 Run/Task/Dispatch/coordinator 字段、旧条件及 host 死函数，校准当前能力汇总；保留准备/恢复/流程机制。[Spec](retired-orca-identity-cleanup.spec.md) · [Plan](retired-orca-identity-cleanup.plan.md) · [专属验收](../acceptance/retired-orca-identity-cleanup.md)。

## 状态命名收敛（普通开发规格）

仅将候选快照及状态事件的 step_id/status 改为 workflow_stage/workflow_status，删除 finish_task_status；其余状态、内部步骤字段和布局保持。[Spec](state-naming.spec.md) · [Plan](state-naming.plan.md) · [专属验收](../acceptance/state-naming.md)。普通开发，不纳入 roadmap，无旧记录兼容；实际结果以专属验收为准。

## 本轮会话与恢复精简

第四批已完成工作区实现及最小自动验收，待 PR 评审；未创建外部 Task/DAG。中心只保存完整 final、问题与必要事实，恢复只继续原会话；多后端、多候选及反馈格式保持。旧构建的故障与实际验证范围由专属验收记录区分。

[Spec](product-simplification-04-conversation-recovery.spec.md) · [Plan](product-simplification-04-conversation-recovery.plan.md) · [验收](../acceptance/product-simplification-04-conversation-recovery.md)

## 停机对账修复

#87 Worker 停机对账与恢复一致性修复：由安装现场 101 的停止操作未知引出的系统性修复，已由本聊天直接交付，[PR #260](https://github.com/erranliu/SteerLine/pull/260) 已真实合并，WSR-A1–A8 自动通过；精确验证范围见专属记录。由 worker 声明并冻结停服行为，本地停止/Orca 继续；应用恢复只消费 worker 规范事实，覆盖原运行、待答和离线完成。复用既有分类/幂等，与 #84 共享接线有序集成，不改变正式排期。

[草稿](../roadmap/drafts/worker-stop-recovery.md) · [Spec](worker-stop-recovery.spec.md) · [Plan](worker-stop-recovery.plan.md) · [专属验收](../acceptance/worker-stop-recovery.md)

## 已确认 Codex 执行规划

#83/#84/#45/#44 均已实现并合入，下面只保留已交付任务的规格及原依赖导航，当前没有由本表派发的剩余任务。实际待验与当前能力限制见 execution-map 和专属 acceptance，不由历史 Spec 推定最新构建已通过。二期验收分[2.1/2.2/2.3](../acceptance/phase2.md)，流程/原型未定部分不提前冻结。

| 任务 ID | 任务名称 | 剩余拟议前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | [Spec](local-runtime-isolation.spec.md) | [Plan](local-runtime-isolation.plan.md) |
| 84 | Worker 分步执行与结构化状态推进 | 无；#83 已进入主干 | [Spec](worker-staged-flow.spec.md) | [Plan](worker-staged-flow.plan.md) |
| 45 | 本机客户端协议与历史查询 | 无 | [Spec](phase2-local-api.spec.md) | [Plan](phase2-local-api.plan.md) |
| 44 | WinUI 基座与原生输入 | #45 | [Spec](phase2-winui-foundation.spec.md) | [Plan](phase2-winui-foundation.plan.md) |

#84 的流程正文集中于独立 Spec，配套 Plan 与[专属验收](../acceptance/worker-staged-flow.md)，不维护重复设计草稿。开发流程为 implement 执行/报告两轮、publish 单轮，复用现有 worker 和 adapter。

## 下一批2.1页面候选

纳入执行前确认2.1最小原型，补齐精确页面测试命令。#49移除对#48的草稿依赖，只交付DAG/任务浏览；不宣称外部边已更改。

| 任务 ID | 任务名称 | 剩余拟议前置 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 49 | DAG 与任务原生浏览 | #44、#45；2.1原型确认 | [Spec](phase2-roadmap-ui.spec.md) | [Plan](phase2-roadmap-ui.plan.md) |
| 50 | 执行与待处理原生界面 | #44、#45；2.1原型确认 | [Spec](phase2-execution-ui.spec.md) | [Plan](phase2-execution-ui.plan.md) |

## 2.2/2.3待重定参考

下列旧正文不可直接准入；流程/模型/原型成熟后重新固定范围、依赖、实现测试计划和专属验收。新人工事项/artifact能力先推导，不强行套入旧发布任务。

| 任务 ID | 任务名称 | 处理 | Spec | Plan |
| --- | --- | --- | --- | --- |
| 46 | 规划提案与受控草稿 | 2.2自由草稿/快照待重定 | [Spec](phase2-planning-drafts.spec.md) | [Plan](phase2-planning-drafts.plan.md) |
| 48 | 正式文档发布与任务创建 | 2.2/2.3审核/授权准入待重定 | [Spec](phase2-document-publish.spec.md) | [Plan](phase2-document-publish.plan.md) |
| 52 | 文档审阅与确认原生界面 | 旧发布页面待重定 | [Spec](phase2-review-ui.spec.md) | [Plan](phase2-review-ui.plan.md) |
| 47 | Codex 规划会话服务 | 2.3接入及必要能力验证待重定 | [Spec](phase2-planning-sessions.spec.md) | [Plan](phase2-planning-sessions.plan.md) |
| 51 | 规划对话原生界面 | 2.3会话/草稿交互待重定 | [Spec](phase2-conversation-ui.spec.md) | [Plan](phase2-conversation-ui.plan.md) |

#43“Codex规划主agent能力验证”保留身份及[旧Spec](phase2-agent-capabilities.spec.md)/[旧Plan](phase2-agent-capabilities.plan.md)/[旧验收](../acceptance/archive/superseded/phase2-agent-capabilities.md)，不再安排独立完整探测；必要门槛纳入#47产品接入。不是对外部Task的取消操作。

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
| local Codex app-server | [Spec](local-codex-app-server.spec.md) · [Plan](local-codex-app-server.plan.md) | A/B/C 已完成，定向 race detector 仍受工具链条件限制；见[状态](../acceptance/local-codex-app-server.md) |
| 本机 Orca 原生编排 | [Spec](worker-flow-orca-native.spec.md) · [Plan](worker-flow-orca-native.plan.md) | 活动轮次输入、消息竞争/故障、完整输出和独立生命周期仍按原 ON 场景核验；见[验收边界](../acceptance/worker-flow-orca-native.md) |
| worker 停服恢复 | [Spec](worker-service-recovery.spec.md) · [Plan](worker-service-recovery.plan.md) | local 闭环已验证，Orca 新 agent 安全重建仍缺真机证据；见[验证记录](../acceptance/worker-service-recovery.md) |

## 普通开发重构规划

后端组装与最终回复状态两项收紧、合并为一份[Spec](bounded-structure-refactoring.spec.md) / [Plan](bounded-structure-refactoring.plan.md)。2026-10-10 基于 `21d97350` 实施并在质量自审后集成 `8e292172`，自动验证及 PR/独立复审状态见[专属记录](../acceptance/bounded-structure-refactoring.md)。不分配/派发任务编号、不进入 execution-map；执行器提取暂缓。审计及旧编号追溯见[范围收敛记录](../roadmap/drafts/bounded-refactoring.md)。

## Orca 单终端原生状态切换（普通开发）

只根据原主 agent 的可信状态切换确认执行开始/结束，缺信号时补一次查询，普通回复只用于结果采集。保留原终端和统一提示词，不使用 supervised、模型标记或结构化对话。已实施，自动验证与真实交付范围见[专属验收](../acceptance/orca-native-state-transitions.md)。[Spec](orca-native-state-transitions.spec.md) · [Plan](orca-native-state-transitions.plan.md)。

## Orca 本机输入与就绪（普通开发）

配对普通终端、等待预算与拒绝回执分类；两个只读回合及一个本机真实合并交付已通过。[Spec](orca-local-input-readiness.spec.md) · [Plan](orca-local-input-readiness.plan.md) · [验收](../acceptance/orca-local-input-readiness.md)。

## 共同约束

- [执行架构](execution-architecture.spec.md)：当前持续 worker、单写者、身份、反馈与真实交付边界。
- [原生任务](native-task-constraints.md)：固定契约、DAG、唯一权威和受控命令。
- [第一阶段](phase1-delivery-constraints.md)：本机候选、安装/调试隔离与单安装版组合验收交接。
- [第二阶段](phase2-constraints.md)：受控规划、文档发布、原生 UI 和公共传输。

当前实现与模块边界见[代码架构](../architecture/current-code-architecture.md)，测试选择见[测试指南](../testing.md)。已完成的执行隔离、主包解耦与三线迁移规格及其阶段约束一并归档，不再列作活动执行入口。

## 人工为主的阶段验收

原 #42/#53 不作为执行任务候选；身份保留历史，原 Spec/Plan 已退役。自动与人工行为、通过条件和状态分别见[一期验收](../acceptance/phase1.md)及[二期验收](../acceptance/phase2.md)。自动验收可独立成任务或进入实施 Plan，具体行为和记录都链接 acceptance。
