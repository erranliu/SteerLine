# 当前 Spec 与 Plan

SteerRunner 拆分后的测试精简（2026-10-15，普通开发，已授权实施）：[Spec](runner-test-convergence.spec.md) · [Plan](runner-test-convergence.plan.md) · [专属验收](../acceptance/runner-test-convergence.md)。执行专项归 Runner；L2 保留本机 Codex 真实实现、报告、发布、SteerlineTest PR 合并和 Task 交付全流程，不改 roadmap。

TUI 退役与 Web 唯一交互前端（普通开发）：[Spec](tui-retirement.spec.md) · [Plan](tui-retirement.plan.md) · [专属验收](../acceptance/tui-retirement.md)。2026-10-15 按用户授权实施，实时结果归专属验收；包含默认 Web 调试包、共用代码保留与 TUI 专属测试清理，不改 roadmap。

本目录保留独立 Spec/Plan、未闭合的定向验收和共同约束。本轮 #91/#92/#93 已交付，#94/#95/#96继续留草稿，范围及结果见[正式执行清单](../roadmap/execution-map.md)与[草稿](../roadmap/drafts/roadmap.md)。#83/#84/#45/#44 的历史交付事实保留；一期及 #84 原人工项继续待验，#44 后续人工要求已撤销，原未验不标通过。旧二阶段及延期候选退出规划，编号不复用。派发仍须核对用户授权、任务状态和固定文档版本。

Spec 中的验收表定义要求，Plan 定义实施及最小测试安排；具体自动/人工步骤与实时已验/待验结果统一维护于[acceptance](../acceptance/README.md)。草稿阶段就建立任务专属的一份或多份验收文档并链接，单份文档不覆盖多任务；Spec/Plan 后续细化并绑定场景 ID。execution-map 在纳入执行或实际验收安排后同步验收汇总及未验文档。旧计划中的 docs/validation 路径只用于定位原证据，不作为新结果写入入口：新原始日志用 runner 默认临时目录或本次新建的空临时目录，长期记录只提交 acceptance 的最小摘要。人工为主活动不作为执行任务；少量人工项按允许范围后置。

已执行的 Spec 与配套 Plan 见[已交付归档](../archive/spec/completed/README.md)；退役流程和远端试验见[历史资料](../archive/spec/retired/README.md)。归档中的旧状态文字和路径属于原交付时点，不能当作当前待办。任务 ID 身份记录保留；原始验证数据从[清理前固定 Git 快照](../acceptance/archive/completed/history.md#历史取证)读取。

## 产品正式文档生成规范

本机 Web 前端由用户直接授权普通开发，独立项目路径 `web/`：[Spec](web-frontend.spec.md) · [Plan](web-frontend.plan.md) · [专属验收](../acceptance/web-frontend.md)。不修改已有 roadmap 任务准入。

SSE 前的 Web 查询优化（2026-10-15，用户已授权实施）：统一 Query 缓存、有界基础预取、缓存展示、可见区域查询和操作后定向失效，不增加同步框架或后端接口。[Spec](web-query-optimization.spec.md) · [Plan](web-query-optimization.plan.md) · [专属验收](../acceptance/web-query-optimization.md)。普通开发，不纳入 roadmap；后续 SSE 沿用同一查询缓存。

2026-10-15 普通开发批次如下；独立 Web 与项目切换已有实现及验收记录，统一启动与多项目宿主为后继重构规划，项目添加继续后置。新增规划不分配 Task ID、不改 execution-map：

| 批次 | 范围与依赖 | 文档 | 验收 |
| --- | --- | --- | --- |
| 先行：独立 Web 与已有项目切换 | 双 exe、Go 应用目录/路由、单端口、当前项目与多标签隔离；仅现有配置，不等待添加流程 | [Spec](web-project-switch.spec.md) · [Plan](web-project-switch.plan.md) | [记录](../acceptance/web-project-switch.md) |
| 重构：统一启动与多项目共进程宿主 | 一个 bat、双常驻进程、后端自动加载全部配置项目；替换独立项目进程，不支持 TUI；仅规划 | [Spec](unified-startup.spec.md) · [Plan](unified-startup.plan.md) | [记录](../acceptance/unified-startup.md) |
| 后续：项目准备、核验与纳管 | 消费先行应用能力及后继宿主；外部 Codex 提示词、结构检查、确认后登记/建库；用户明确后置 | [Spec](project-onboarding.spec.md) · [Plan](project-onboarding.plan.md) | [记录](../acceptance/project-onboarding.md) |

[SpecGuide.md](SpecGuide.md) 维护 SteerLine 产品草稿转正时必读的 Spec / Plan 生成规范：避免过度设计，先查现有代码再整合，先做相关任务整体规划，明确执行链路和桥接必要性，并落实最小测试及验收覆盖。[交互设计](../product-design/planning-ui.md)与[原型](../product-design/prototypes/planning-ui/index.html)展示规范入口和受控生成。当前产品流程为原型设计，不改变用户直接授权的普通仓库开发入口。

## HTTP 接入后的解耦规格

2026-10-14 用户确认#92/#93纳入execution-map；#92已独立复审、验证并经PR #311真实合并，#93已独立复审、验证并经PR #312真实合并，其余三项因正在实施的Worker三层重构影响继续留草稿。五项独立Spec/Plan与专属验收保留，已交付#92/#93移出草稿任务表。任务范围、共享所有者及集成建议见[本轮草稿](../roadmap/drafts/roadmap.md)。

| 任务 ID | 任务名称 | Spec / Plan | 验收 |
| --- | --- | --- | --- |
| 92 | HTTP 服务端命令边界收敛 | [Spec](http-command-boundary.spec.md) · [Plan](http-command-boundary.plan.md) | [记录](../acceptance/http-command-boundary.md) |
| 93 | HTTP 客户端原请求与停服观察解耦 | [Spec](http-client-request-lifecycle.spec.md) · [Plan](http-client-request-lifecycle.plan.md) | [记录](../acceptance/http-client-request-lifecycle.md) |
| 94 | 固定开发流程规则独立 | [Spec](development-flow-rules.spec.md) · [Plan](development-flow-rules.plan.md) | [记录](../acceptance/development-flow-rules.md) |
| 95 | 任务最终交付用例独立 | [Spec](task-delivery-usecase.spec.md) · [Plan](task-delivery-usecase.plan.md) | [记录](../acceptance/task-delivery-usecase.md) |
| 96 | worker 运行时资源所有权收敛 | [Spec](worker-runtime-ownership.spec.md) · [Plan](worker-runtime-ownership.plan.md) | [记录](../acceptance/worker-runtime-ownership.md) |

#94的Workflow规则、#96的生命周期/资源接线与三层方案交叉；#95交付抽取未被取代但actor/PR桥接正在重组，暂不安排。具体依据见[冲突核对](../roadmap/drafts/roadmap.md#与worker三层重构的冲突核对)；本表是资料导航，不用于自动派发。

## Worker 三层职责重构（普通开发规格）

已按用户直接授权实施并完成约定自动验收；最新收尾 rebase 的复验已通过，实际版本与证据见专属验收。Workflow 管业务步骤，ReliableExecution 管通用可靠执行，ExecutionAdapter 管原生动作/证据；两条路径一起接入，保留单写提交与现有行为。初始化失败重试等新增能力单独留出，不接管安装任务 22。

[Spec](worker-three-layer.spec.md) · [Plan](worker-three-layer.plan.md) · [专属验收](../acceptance/worker-three-layer.md) · [长期流程规则](../product-design/worker-lifecycle.md)。普通开发，无 Task ID，不修改 execution-map。

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

## 既有规划与历史交付

本轮 #91 已经 [PR #299](https://github.com/erranliu/SteerLine/pull/299) 真实合并；#92已独立复审、自动复验并经 [PR #311](https://github.com/erranliu/SteerLine/pull/311) 合并，#93已独立复审、验证并经PR #312真实合并，范围/并行条件见execution-map；#94/#95/#96尚未纳入。#91独立 SteerlineTest live/真终端操作已验，剩余人工体验见[验收](../acceptance/tui-http-client.md)。旧二阶段候选/三节点与延期版本任务退出活动入口，编号保留，材料见[归档](../archive/roadmap/README.md)。#44 实现已由 #91 删除，#45 保留已交付基线。

| 任务 ID | 任务名称 | 前置 | Spec | Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 91 | TUI 经 HTTP 接入 Go 后端（后续已退役） | 无未完成硬前置；复用 #45 与现有 TUI | [Spec](tui-http-client.spec.md) | [Plan](tui-http-client.plan.md) | [记录](../acceptance/tui-http-client.md) |

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

## Orca 单终端原生状态切换（普通开发）

只根据原主 agent 的可信状态切换确认执行开始/结束，缺信号时补一次查询，普通回复只用于结果采集。保留原终端和统一提示词，不使用 supervised、模型标记或结构化对话。已实施，自动验证与真实交付范围见[专属验收](../acceptance/orca-native-state-transitions.md)。[Spec](orca-native-state-transitions.spec.md) · [Plan](orca-native-state-transitions.plan.md)。

## Orca 本机输入与就绪（普通开发）

配对普通终端、等待预算与拒绝回执分类；两个只读回合及一个本机真实合并交付已通过。[Spec](orca-local-input-readiness.spec.md) · [Plan](orca-local-input-readiness.plan.md) · [验收](../acceptance/orca-local-input-readiness.md)。

## Codex / Orca 断连恢复规划

2026-10-12 经用户要求审计后收敛为可一次实施的首轮设计：原观察重接、明确失败的继续入口、Codex 原进程退出后同 thread 恢复，人工成果复用 implement 重核；Orca 先支持原终端重接与资源丢失诊断。专用成果接纳、远端真实故障链与全窗口增强延后。用户已授权按收敛范围实施，工作树实现及本轮自动验收已完成，未部署，不部署 Runtime；普通开发，无 Task ID，不进入 execution-map。

[Spec](worker-disconnect-recovery.spec.md) · [Plan / 测试计划](worker-disconnect-recovery.plan.md) · [专属验收](../acceptance/worker-disconnect-recovery.md) · [L3 场景设计](../../tests/e2e/suites/recovery/README.md)。场景已补实现/注册，实时执行状态见专属验收。

## 共同约束

- [执行架构](execution-architecture.spec.md)：当前持续 worker、单写者、身份、反馈与真实交付边界。
- [原生任务](native-task-constraints.md)：固定契约、DAG、唯一权威和受控命令。
- [第一阶段](phase1-delivery-constraints.md)：本机候选、安装/调试隔离与单安装版组合验收交接。
- [第二阶段](phase2-constraints.md)：原 TUI/HTTP/Go 边界历史；公共 API 部分继续有效。

当前实现与模块边界见[代码架构](../architecture/current-code-architecture.md)，测试选择见[测试指南](../testing.md)。已完成的执行隔离、主包解耦与三线迁移规格及其阶段约束一并归档，不再列作活动执行入口。

## 人工为主的阶段验收

原 #42/#53 不作为执行任务候选；身份保留历史，原 Spec/Plan 已退役。自动与人工行为、通过条件和状态分别见[一期验收](../acceptance/phase1.md)及[二期验收](../acceptance/phase2.md)。自动验收可独立成任务或进入实施 Plan，具体行为和记录都链接 acceptance。
