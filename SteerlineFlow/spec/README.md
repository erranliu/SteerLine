# 当前 Spec 与 Plan

候选停止可靠受理与核实（2026-10-25，自审收敛后实施）：[Spec](candidate-stop-reliability.spec.md) · [Plan](../plans/candidate-stop-reliability.plan.md) · [专属验收](../testing/acceptance/candidate-stop-reliability.md)。停止不因无关候选/派发版本增长失效，原请求/退出事实恢复、原 actor 及时让路、有界独立核实及失败/未知/快照刷新分开呈现；保留最后一个、交付竞争和资源边界。不改 roadmap 或安装现场。

Runner 准备查询等待与执行页展示（2026-10-25，调用方实施、远端真机及回归通过）：[Spec](runner-wait-retry.spec.md) · [Plan](../plans/runner-wait-retry.plan.md) · [专属验收](../testing/acceptance/runner-wait-retry.md)。Runner 独立实现两项准备读取的有界等待/重试与最小公开契约；本仓消费原投影/活动并展示，未知副作用保持原规则。正式 schema 的 L1/L2/L3、Web 及远端 Orca 同候选恢复/停止已核实；历史告警、失败轮和未验边界分别留证，不恢复生产现场或部署。

项目更新与候选版本收敛（2026-10-21，已实施）：[Spec](project-maintenance-simplification.spec.md) · [Plan](../plans/project-maintenance-simplification.plan.md) · [专属验收](../testing/acceptance/project-maintenance-simplification.md) · [长期设计](../../docs/product-design/project-maintenance.md)。远端/本地提交与PR、日志及一键快进，材料自动跟随、Plan变化自动生成系统测试安排；每个新候选固定最新版本，旧候选过期保留且禁用当前测试关联，其PR合入仍可完成任务。取代下述上一批人工确认/应用及Task级冻结；实际验证见专属验收，不改roadmap。

项目基线与材料统一维护（2026-10-21）：[Spec](project-maintenance.spec.md) · [Plan](../plans/project-maintenance.plan.md) · [专属验收](../testing/acceptance/project-maintenance.md) · [长期设计](../../docs/product-design/project-maintenance.md)。初始化/基线/局部共用规则和维护流程，后台检测零自动生成，生成先审阅再显式应用，首次启动冻结远端最新代码与同版本材料。当前已替换任务材料登记即永久固定、生成自动保存及缺材料仅提示的旧行为；验证归专属记录，不改历史验收或 roadmap。项目添加的最终登记/建库继续后置，消费本批共用检查能力。

Web 执行配置管理与能力探测（2026-10-21，按确认原型实施）：[Spec](web-execution-config.spec.md) · [Plan](../plans/web-execution-config.plan.md) · [专属验收](../testing/acceptance/web-execution-config.md)。Web 前后端直接查询 Codex/Orca，管理全局预设及条件保存；保存后显式重启生效。Go 执行服务只消费配置和投影加载摘要，Runner 不新增探测接口；实现与验证范围登记于专属验收，不修改 roadmap。本机 Orca 已实现；目录查询统一本机/远端Orca能力，与执行支持组合分开。provider凭据由原Agent管理，Pi按已有目录筛选Provider；验证见专属验收。

Runner generate/1 对接与最简测试管理（自动项通过，人工待验）：[Spec](test-management.spec.md) · [Plan](../plans/test-management.plan.md) · [专属验收](../testing/acceptance/test-management.md)。公开产物收集、当前 Git 核验、生成对象兼容接管及真实结果闭环已接入；TM-A04/TM-A08 等实际版本与结果以专属记录为准，TM-M01 后置。生成先审阅再显式应用等后续变更归[项目维护](../../docs/product-design/project-maintenance.md)，原固定版本证据保持。不增加 Task 字段/测试门禁或 conversation 操作。

应用级共享 SteerRunner 服务（2026-10-19，已实施）：[Spec](shared-runner-service.spec.md) · [Plan](../plans/shared-runner-service.plan.md) · [专属验收](../testing/acceptance/shared-runner-service.md)。每个 SteerLine home 共用一个 Runner，服务所有权上移到应用；覆盖跨项目请求身份、环境检查、停机证明、换代与整包 reset，仅修改 SteerLine。固定 L1、定向反例、两个 L3 及新包真实空闲拓扑验证通过，人工体验待验；后续[闪窗修复](../testing/acceptance/startup-console-windows.md)已有独立记录，不修改 roadmap。

SteerLine / SteerRunner 协作与多 Workflow 接入（2026-10-17，已授权实施）：[Spec](runner-collaboration.spec.md) · [Plan](../plans/runner-collaboration.plan.md) · [专属验收](../testing/acceptance/runner-collaboration.md)。多 workflow、分层冻结、构建 pin 及产品投影已接入；U1 验证旧运行时拒绝替代实例、显式重启后接纳及旧停止隔离。实际固定构建、真实链结果和人工待验以 acceptance 为准；不部署安装环境或修改 roadmap。

旧执行记录与空壳字段清理（2026-10-17，普通开发）：[Plan](../plans/runtime-shell-cleanup.plan.md) · [专属验收](../testing/acceptance/runtime-shell-cleanup.md)。删除无消费者的旧记录、内部字段及辅助函数，校准当前文档；保留现有读取链和模型归属，不改 roadmap。

产品读取、命令入口与 Web 容器重构（2026-10-16，按审计收窄并实施）：[Spec](product-read-command-web-refactor.spec.md) · [Plan](../plans/product-read-command-web-refactor.plan.md) · [专属验收](../testing/acceptance/product-read-command-web-refactor.md)。优先收拢候选事实/展示规则；命令保留唯一仲裁与三种受理顺序，Web 提取副作用 hooks；不重建整套读取/命令/前端框架，不改 roadmap。

应用与项目生命周期收拢（2026-10-16，普通开发，按审计收窄并实施）：[Spec](application-project-lifecycle.spec.md) · [Plan](../plans/application-project-lifecycle.plan.md) · [验收](../testing/acceptance/application-project-lifecycle.md)。删除旧启动链，统一项目资源 owner、部分启动收尾和进程内调用排空；保留原停机/失败策略，不改 roadmap。

执行权限与 Git 环境检查接入（已实现，正式公开契约已对齐）：[Spec](environment-check.spec.md) · [Plan](../plans/environment-check.plan.md) · [本仓验收与审查记录](../testing/acceptance/environment-check.md)。caller/HTTP/Web 自动验证及固定版本本机 full_access 真实正例已有记录，启动配置与报告刷新修复已合入；原生正例仅证明记录中的源码/Runner 版本和模式，不代表全部权限、远端或恢复组合通过。SLEC-M01 完整人工体验仍待安排，Runner 其余专项归其独立 SREC 验收；构建及业务资源后置，不改 roadmap。

SteerRunner 拆分后的测试精简（2026-10-15，普通开发，已实施）：[Spec](runner-test-convergence.spec.md) · [Plan](../plans/runner-test-convergence.plan.md) · [专属验收](../testing/acceptance/runner-test-convergence.md)。执行专项归 Runner；L2 保留本机 Codex 真实实现、报告、发布、SteerlineTest PR 合并和 Task 交付全流程，不改 roadmap。

TUI 退役与 Web 唯一交互前端（普通开发）：[Spec](tui-retirement.spec.md) · [Plan](../plans/tui-retirement.plan.md) · [专属验收](../testing/acceptance/tui-retirement.md)。2026-10-15 按用户授权实施，实时结果归专属验收；包含默认 Web 调试包、共用代码保留与 TUI 专属测试清理，不改 roadmap。

本目录保留独立 Spec/Plan、未闭合的定向验收和共同约束。#91/#92/#93 已交付；#91 的 TUI 客户端已由后续普通开发退役，Web 是当前唯一交互前端。#94/#95/#96继续留草稿，范围及结果见[正式执行清单](../Roadmap/execution-map.md)与[草稿](../Roadmap/drafts/roadmap.md)。#83/#84/#45/#44 的历史交付事实保留；一期及 #84 原人工项继续待验，#44 后续人工要求已撤销，原未验不标通过。旧二阶段及延期候选退出规划，编号不复用。派发仍须核对用户授权、任务状态和固定文档版本。

Spec 中的验收表定义要求，Plan 定义实施及最小测试安排；具体自动/人工步骤与实时已验/待验结果统一维护于[acceptance](../testing/acceptance/README.md)。草稿阶段就建立任务专属的一份或多份验收文档并链接，单份文档不覆盖多任务；Spec/Plan 后续细化并绑定场景 ID。execution-map 在纳入执行或实际验收安排后同步验收汇总及未验文档。旧计划中的 SteerlineFlow/testing/validation 路径只用于定位原证据，不作为新结果写入入口：新原始日志用 runner 默认临时目录或本次新建的空临时目录，长期记录只提交 acceptance 的最小摘要。人工为主活动不作为执行任务；少量人工项按允许范围后置。

已执行的 Spec 与配套 Plan 见[已交付归档](../archive/spec/completed/README.md)；退役流程和远端试验见[历史资料](../archive/spec/retired/README.md)。归档中的旧状态文字和路径属于原交付时点，不能当作当前待办。任务 ID 身份记录保留；原始验证数据从[清理前固定 Git 快照](../testing/acceptance/archive/completed/history.md#历史取证)读取。

## 产品正式文档生成规范

本机 Web 前端由用户直接授权普通开发，独立项目路径 `web/`：[Spec](web-frontend.spec.md) · [Plan](../plans/web-frontend.plan.md) · [专属验收](../testing/acceptance/web-frontend.md)。不修改已有 roadmap 任务准入。

SSE 前的 Web 查询优化（2026-10-15，用户已授权实施）：统一 Query 缓存、有界基础预取、缓存展示、可见区域查询和操作后定向失效，不增加同步框架或后端接口。[Spec](web-query-optimization.spec.md) · [Plan](../plans/web-query-optimization.plan.md) · [专属验收](../testing/acceptance/web-query-optimization.md)。普通开发，不纳入 roadmap；后续 SSE 沿用同一查询缓存。

2026-10-15 普通开发批次如下；独立 Web、项目切换及统一启动宿主已有实现和验收记录，桌面体验待人工验收；项目添加继续后置。新增规划不分配 Task ID、不改 execution-map：

| 批次 | 范围与依赖 | 文档 | 验收 |
| --- | --- | --- | --- |
| 先行：独立 Web 与已有项目切换 | 双 exe、Go 应用目录/路由、单端口、当前项目与多标签隔离；仅现有配置，不等待添加流程 | [Spec](web-project-switch.spec.md) · [Plan](../plans/web-project-switch.plan.md) | [记录](../testing/acceptance/web-project-switch.md) |
| 重构：统一启动与多项目共进程宿主 | 一个 bat、双常驻进程、后端加载配置项目；已实施并完成约定自动验收，桌面体验待人工验收 | [Spec](unified-startup.spec.md) · [Plan](../plans/unified-startup.plan.md) | [记录](../testing/acceptance/unified-startup.md) |
| 后续：项目准备、核验与纳管 | 消费先行应用能力及后继宿主；外部 Codex 提示词、结构检查、确认后登记/建库；用户明确后置 | [Spec](project-onboarding.spec.md) · [Plan](../plans/project-onboarding.plan.md) | [记录](../testing/acceptance/project-onboarding.md) |

[SpecGuide.md](SpecGuide.md) 维护 SteerLine 产品草稿转正时必读的 Spec / Plan 生成规范：避免过度设计，先查现有代码再整合，先做相关任务整体规划，明确执行链路和桥接必要性，并落实最小测试及验收覆盖。[早期交互设计归档](../archive/design/planning-ui.md)与[原型](../../docs/product-design/prototypes/planning-ui/index.html)记录规范入口和受控生成设想，不表示对应产品流程已实现，也不改变用户直接授权的普通仓库开发入口。

## HTTP 接入后的解耦规格

2026-10-14 用户确认#92/#93纳入execution-map；#92已独立复审、验证并经PR #311真实合并，#93已独立复审、验证并经PR #312真实合并，其余三项仍留草稿；原冲突判断形成于三层重构实施期间，当前须结合已合入重构和 Runner 迁出重新核对。五项独立Spec/Plan与专属验收保留，已交付#92/#93移出草稿任务表。任务范围、共享所有者及集成建议见[本轮草稿](../Roadmap/drafts/roadmap.md)。

| 任务 ID | 任务名称 | Spec / Plan | 验收 |
| --- | --- | --- | --- |
| 92 | HTTP 服务端命令边界收敛 | [Spec](http-command-boundary.spec.md) · [Plan](../plans/http-command-boundary.plan.md) | [记录](../testing/acceptance/http-command-boundary.md) |
| 93 | HTTP 客户端原请求与停服观察解耦 | [Spec](http-client-request-lifecycle.spec.md) · [Plan](../plans/http-client-request-lifecycle.plan.md) | [记录](../testing/acceptance/http-client-request-lifecycle.md) |
| 94 | 固定开发流程规则独立 | [Spec](development-flow-rules.spec.md) · [Plan](../plans/development-flow-rules.plan.md) | [记录](../testing/acceptance/development-flow-rules.md) |
| 95 | 任务最终交付用例独立 | [Spec](task-delivery-usecase.spec.md) · [Plan](../plans/task-delivery-usecase.plan.md) | [记录](../testing/acceptance/task-delivery-usecase.md) |
| 96 | worker 运行时资源所有权收敛 | [Spec](worker-runtime-ownership.spec.md) · [Plan](../plans/worker-runtime-ownership.plan.md) | [记录](../testing/acceptance/worker-runtime-ownership.md) |

#94/#95/#96 的原拆分针对迁出前代码；三层重构已合入，执行随后迁至 Runner，原范围和收益须重核，当前仍未纳入、不自动启动。历史依据见[冲突核对](../Roadmap/drafts/roadmap.md#与worker三层重构的冲突核对)；本表是资料导航，不用于自动派发。

## 已交付规格归档

控制台主界面、四批产品精简和迁出前 Worker 三层重构已交付，其 Spec/Plan 与原版本验收由[已交付归档](../archive/spec/completed/README.md#2026-10-21-补充归档)承接。三层重构经 PR #313 合入，执行实现随后迁至独立 Runner；长期流程规则继续维护于 [Worker 生命周期](../../docs/product-design/worker-lifecycle.md)。已退役的 TUI 和本机 Orca 方案不再列作当前实施或补验入口。

## 旧 Orca 编排身份残留清理（普通开发规格）

删除当前后端不再使用的 Run/Task/Dispatch/coordinator 字段、旧条件及 host 死函数，校准当前能力汇总；保留准备/恢复/流程机制。[Spec](retired-orca-identity-cleanup.spec.md) · [Plan](../plans/retired-orca-identity-cleanup.plan.md) · [专属验收](../testing/acceptance/retired-orca-identity-cleanup.md)。

## 状态命名收敛（普通开发规格）

仅将候选快照及状态事件的 step_id/status 改为 workflow_stage/workflow_status，删除 finish_task_status；其余状态、内部步骤字段和布局保持。[Spec](state-naming.spec.md) · [Plan](../plans/state-naming.plan.md) · [专属验收](../testing/acceptance/state-naming.md)。普通开发，不纳入 roadmap，无旧记录兼容；实际结果以专属验收为准。

## 会话与恢复精简历史

第四批产品精简已由 PR #243 合入；[Spec / Plan](../archive/spec/completed/README.md#2026-10-21-补充归档)及[原版本验收](../testing/acceptance/archive/completed/product-simplification-04-conversation-recovery.md)已归档。后续恢复规则以 [Worker 生命周期](../../docs/product-design/worker-lifecycle.md)和[服务换代恢复](service-generation-recovery.spec.md)为准。

## 停机对账修复

#87 Worker 停机对账与恢复一致性修复：由安装现场 101 的停止操作未知引出的系统性修复，已由本聊天直接交付，[PR #260](https://github.com/erranliu/SteerLine/pull/260) 已真实合并，WSR-A1–A8 自动通过；精确验证范围见专属记录。由 worker 声明并冻结停服行为，本地停止/Orca 继续；应用恢复只消费 worker 规范事实，覆盖原运行、待答和离线完成。复用既有分类/幂等，与 #84 共享接线有序集成，不改变正式排期。

[草稿](../Roadmap/drafts/worker-stop-recovery.md) · [Spec](worker-stop-recovery.spec.md) · [Plan](../plans/worker-stop-recovery.plan.md) · [专属验收](../testing/acceptance/worker-stop-recovery.md)

## 既有规划与历史交付

任务 #91 已由 PR #299 合入，#92/#93 分别由 PR #311/#312 合入；#91 保留了 TUI，之后 TUI 已通过独立普通开发任务退役，详见[退役验收](../testing/acceptance/tui-retirement.md)。#94/#95/#96 尚未纳入，范围/并行条件见[执行清单](../Roadmap/execution-map.md)和[草稿](../Roadmap/drafts/roadmap.md)。旧二阶段候选/三节点与延期版本任务退出活动入口，编号保留，材料见[归档](../archive/roadmap/README.md)。#44 实现由 #91 删除，#45 保留为历史 HTTP 基线。

| 任务 ID | 任务名称 | 前置 | Spec | Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 91 | TUI 经 HTTP 接入 Go 后端（TUI 后续已退役） | 无未完成硬前置；复用 #45 与当时的 TUI | [Spec](tui-http-client.spec.md) | [Plan](../plans/tui-http-client.plan.md) | [记录](../testing/acceptance/tui-http-client.md) · [后续退役](../testing/acceptance/tui-retirement.md) |

#45 原[Spec](phase2-local-api.spec.md)/[Plan](../plans/phase2-local-api.plan.md)保留历史基线，扩展按 #91；#83/#84/#87 已交付与当前未验按[execution-map](../Roadmap/execution-map.md)核对，不重新派发。

## 尚未闭合的定向验收

下列记录保留原交付范围内的验证缺口；迁出前验收不直接代表当前 Runner 能力，是否补验须按现行契约重核适用性，不重新创建任务身份。本机 Orca 原编排已退役，其未验项转入历史记录。

| 范围 | Spec / Plan | 剩余边界与证据 |
| --- | --- | --- |
| local Codex app-server | [Spec](local-codex-app-server.spec.md) · [Plan](../plans/local-codex-app-server.plan.md) | A/B/C 已完成，定向 race detector 仍受工具链条件限制；见[状态](../testing/acceptance/local-codex-app-server.md) |
| worker 停服恢复 | [Spec](worker-service-recovery.spec.md) · [Plan](../plans/worker-service-recovery.plan.md) | local 闭环已验证，Orca 新 agent 安全重建仍缺真机证据；见[验证记录](../testing/acceptance/worker-service-recovery.md) |

## 普通开发重构规划

后端组装与最终回复状态两项收紧、合并为一份[Spec](bounded-structure-refactoring.spec.md) / [Plan](../plans/bounded-structure-refactoring.plan.md)。2026-10-10 基于 `21d97350` 实施并在质量自审后集成 `8e292172`，自动验证及 PR/独立复审状态见[专属记录](../testing/acceptance/bounded-structure-refactoring.md)。不分配/派发任务编号、不进入 execution-map；执行器提取暂缓。审计及旧编号追溯见[范围收敛记录](../Roadmap/drafts/bounded-refactoring.md)。

## Orca 单终端原生状态切换（普通开发）

只根据原主 agent 的可信状态切换确认执行开始/结束，缺信号时补一次查询，普通回复只用于结果采集。保留原终端和统一提示词，不使用 supervised、模型标记或结构化对话。已实施，自动验证与真实交付范围见[专属验收](../testing/acceptance/orca-native-state-transitions.md)。[Spec](orca-native-state-transitions.spec.md) · [Plan](../plans/orca-native-state-transitions.plan.md)。

## 退役 Orca 方案

本机 Orca 已放弃；supervised、worker-start、Dispatch 和本机输入/就绪方案见[退役规格](../archive/spec/retired/README.md#2026-10-21-orca-历史方案归档)及[原验收](../testing/acceptance/archive/README.md#退役-orca-方案与原版本证据)。历史失败、取消和未验仍保留，不计入当前远端路径已验范围。远端接入与构建要求见 [Runner 接入](../../docs/integration/steerrunner.md)。

## Codex / Orca 断连恢复规划

2026-10-12 经用户要求审计后收敛为可一次实施的首轮设计：原观察重接、明确失败的继续入口、Codex 原进程退出后同 thread 恢复，人工成果复用 implement 重核；Orca 先支持原终端重接与资源丢失诊断。专用成果接纳、远端真实故障链与全窗口增强延后。用户已授权按收敛范围实施，工作树实现及本轮自动验收已完成，未部署，不部署 Runtime；普通开发，无 Task ID，不进入 execution-map。

[Spec](worker-disconnect-recovery.spec.md) · [Plan / 测试计划](../plans/worker-disconnect-recovery.plan.md) · [专属验收](../testing/acceptance/worker-disconnect-recovery.md) · [L3 场景设计](../../tests/e2e/suites/recovery/README.md)。场景已补实现/注册，实时执行状态见专属验收。

## 共同约束

- [执行架构](execution-architecture.spec.md)：当前持续 worker、单写者、身份、反馈与真实交付边界。
- [原生任务](native-task-constraints.md)：固定契约、DAG、唯一权威和受控命令。
- [第一阶段](phase1-delivery-constraints.md)：本机候选、安装/调试隔离与单安装版组合验收交接。
- [第二阶段](phase2-constraints.md)：原 TUI/HTTP/Go 边界历史；公共 API 部分继续有效。

当前实现与模块边界见[代码架构](../../docs/architecture/current-code-architecture.md)，测试选择见[测试指南](../testing.md)。已完成的执行隔离、主包解耦与三线迁移规格及其阶段约束一并归档，不再列作活动执行入口。

## 人工为主的阶段验收

原 #42/#53 不作为执行任务候选；身份保留历史，原 Spec/Plan 已退役。自动与人工行为、通过条件和状态分别见[一期验收](../testing/acceptance/phase1.md)及[二期验收](../testing/acceptance/phase2.md)。自动验收可独立成任务或进入实施 Plan，具体行为和记录都链接 acceptance。
