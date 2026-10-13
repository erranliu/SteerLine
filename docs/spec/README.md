# 当前 Spec 与 Plan

本目录保留独立 Spec/Plan、未闭合的定向验收和共同约束。当前唯一规划任务为 #91，范围见[正式执行清单](../roadmap/execution-map.md)与[草稿](../roadmap/drafts/roadmap.md)。#83/#84/#45/#44 的历史交付事实保留；一期及 #84 原人工项继续待验，#44 后续人工要求已撤销，原未验不标通过。旧二阶段及延期候选退出规划，编号不复用。派发仍须核对用户授权、任务状态和固定文档版本。

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

## 当前已确认规划

当前只有 #91；2026-10-13 用户确认规划并直接授权实现，源码与自动验证已交付，独立 SteerlineTest live/真终端操作已验，PR 待独立复审/合入，剩余人工体验见[验收](../acceptance/tui-http-client.md)。旧二阶段候选/三节点与延期版本任务退出活动入口，编号保留，材料见[归档](../archive/roadmap/README.md)。#44 实现已由 #91 删除，#45 保留已交付基线。

| 任务 ID | 任务名称 | 前置 | Spec | Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 91 | TUI 经 HTTP 接入 Go 后端 | 无未完成硬前置；复用 #45 与现有 TUI | [Spec](tui-http-client.spec.md) | [Plan](tui-http-client.plan.md) | [记录](../acceptance/tui-http-client.md) |

#45 原[Spec](phase2-local-api.spec.md)/[Plan](phase2-local-api.plan.md)保留历史基线，扩展按 #91；#83/#84/#87 已交付与当前未验按[execution-map](../roadmap/execution-map.md)核对，不重新派发。

## 尚未闭合的定向验收

下列实现已有合入基线，但仍有明确的验证缺口；不因归档其他已交付任务而宣告完成，也不重新创建任务身份。

| 范围 | Spec / Plan | 剩余边界与证据 |
| --- | --- | --- |
| local Codex app-server | [Spec](local-codex-app-server.spec.md) · [Plan](local-codex-app-server.plan.md) | A/B/C 已完成，定向 race detector 仍受工具链条件限制；见[状态](../acceptance/local-codex-app-server.md) |
| 本机 Orca 原生编排 | [Spec](worker-flow-orca-native.spec.md) · [Plan](worker-flow-orca-native.plan.md) | 活动轮次输入、消息竞争/故障、完整输出和独立生命周期仍按原 ON 场景核验；见[验收边界](../acceptance/worker-flow-orca-native.md) |
| worker 停服恢复 | [Spec](worker-service-recovery.spec.md) · [Plan](worker-service-recovery.plan.md) | local 闭环已验证，Orca 新 agent 安全重建仍缺真机证据；见[验证记录](../acceptance/worker-service-recovery.md) |

## 普通开发重构规划

后端组装与最终回复状态两项收紧、合并为一份[Spec](bounded-structure-refactoring.spec.md) / [Plan](bounded-structure-refactoring.plan.md)。2026-10-10 基于 `21d97350` 实施并在质量自审后集成 `8e292172`，自动验证及 PR/独立复审状态见[专属记录](../acceptance/bounded-structure-refactoring.md)。不分配/派发任务编号、不进入 execution-map；执行器提取暂缓。审计及旧编号追溯见[范围收敛记录](../roadmap/drafts/bounded-refactoring.md)。

## Orca 单终端原生状态切换（普通开发规划）

只根据原主 agent 的可信状态切换确认执行开始/结束，缺信号时补一次查询，普通回复只用于结果采集。保留原终端和统一提示词，不使用 supervised、模型标记或结构化对话。验证限于本机原生接口样本、定向规则测试及一项受控阶段回归；当前仅规划。[Spec](orca-native-state-transitions.spec.md) · [Plan](orca-native-state-transitions.plan.md) · [专属验收](../acceptance/orca-native-state-transitions.md)。

## 共同约束

- [执行架构](execution-architecture.spec.md)：当前持续 worker、单写者、身份、反馈与真实交付边界。
- [原生任务](native-task-constraints.md)：固定契约、DAG、唯一权威和受控命令。
- [第一阶段](phase1-delivery-constraints.md)：本机候选、安装/调试隔离与单安装版组合验收交接。
- [第二阶段](phase2-constraints.md)：当前 TUI/HTTP/Go 边界；仅 #91 在规划中。

当前实现与模块边界见[代码架构](../architecture/current-code-architecture.md)，测试选择见[测试指南](../testing.md)。已完成的执行隔离、主包解耦与三线迁移规格及其阶段约束一并归档，不再列作活动执行入口。

## 人工为主的阶段验收

原 #42/#53 不作为执行任务候选；身份保留历史，原 Spec/Plan 已退役。自动与人工行为、通过条件和状态分别见[一期验收](../acceptance/phase1.md)及[二期验收](../acceptance/phase2.md)。自动验收可独立成任务或进入实施 Plan，具体行为和记录都链接 acceptance。
