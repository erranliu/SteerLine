# Runner 与可靠性历史证据入口

归档于 2026-11-06。汇集执行、数据、配置、进度、协议、安装/调试及项目维护整理前引用的契约、验收和 Runner 来源，保留各记录的受测版本、失败、通过及未验范围。历史通过不自动赋予当前构建执行或恢复资格；安装状态按实际部署记录判断。

当前说明：[Runner 调用生命周期](../../../docs/product-design/runner-lifecycle.md) · [可靠性交付链路](../../../docs/architecture/reliable-delivery-chain.md) · [Runner 接入](../../../docs/integration/steerrunner.md)。

## 设计来源

未实施简化方案及其原审计锚点见[历史方案](reliability-simplification-analysis.md)。整理前三份原文可在本仓 Git 中按以下固定提交读取：

- 提交：`5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7`。
- `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:docs/product-design/worker-lifecycle.md`。
- `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:docs/architecture/reliable-delivery-chain.md`。
- `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:docs/integration/steerrunner.md`。

## 验收与来源

| 记录 | 使用范围 |
| --- | --- |
| [应用与项目生命周期收拢验收](../../testing/acceptance/application-project-lifecycle.md) | 按原文固定版本、环境和证据范围追溯 |
| [同 Runner 构建的重启与显示更新验收](../../testing/acceptance/application-redeploy.md) | 按原文固定版本、环境和证据范围追溯 |
| [调用方边界收口验收](../../testing/acceptance/architecture-boundary-consolidation.md) | 按原文固定版本、环境和证据范围追溯 |
| [架构解耦第四轮验收：Task 交付类型归位](../../testing/acceptance/architecture-decoupling-round4.md) | 按原文固定版本、环境和证据范围追溯 |
| [架构解耦第五轮验收：宿主 checkpoint 与状态契约](../../testing/acceptance/architecture-decoupling-round5.md) | 按原文固定版本、环境和证据范围追溯 |
| [Worker 三层职责重构验收](../../testing/acceptance/archive/completed/worker-three-layer.md) | 按原文固定版本、环境和证据范围追溯 |
| [用量事实与计费职责划分验收](../../testing/acceptance/billing-ownership.md) | 按原文固定版本、环境和证据范围追溯 |
| [调用方提示词收敛验收记录](../../testing/acceptance/caller-prompt-convergence.md) | 按原文固定版本、环境和证据范围追溯 |
| [验收：候选停止参与的可靠受理与核实](../../testing/acceptance/candidate-stop-reliability.md) | 按原文固定版本、环境和证据范围追溯 |
| [部署自动恢复中断宿主验收](../../testing/acceptance/deployment-interrupted-recovery.md) | 按原文固定版本、环境和证据范围追溯 |
| [执行域遗留清理验收](../../testing/acceptance/execution-legacy-cleanup.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner 执行展示与最近消息验收](../../testing/acceptance/execution-observability.md) | 按原文固定版本、环境和证据范围追溯 |
| [项目维护一键生成与全局队列验收](../../testing/acceptance/generate-queue.md) | 按原文固定版本、环境和证据范围追溯 |
| [新安装目录迁移与部署验收](../../testing/acceptance/install-cutover-20261015.md) | 按原文固定版本、环境和证据范围追溯 |
| [原生执行配置边界收口：验收规划](../../testing/acceptance/native-execution-configuration-boundary.md) | 按原文固定版本、环境和证据范围追溯 |
| [恢复原会话查看验收](../../testing/acceptance/native-session-viewer.md) | 按原文固定版本、环境和证据范围追溯 |
| [Orca 远端原生链路接入验收](../../testing/acceptance/orca-remote-native.md) | 按原文固定版本、环境和证据范围追溯 |
| [Orca 执行主机 runtime 接线验收](../../testing/acceptance/orca-runtime-wiring.md) | 按原文固定版本、环境和证据范围追溯 |
| [Orca 部署重验调用方](../../testing/acceptance/orca-shutdown-verification.md) | 按原文固定版本、环境和证据范围追溯 |
| [待合并 PR 冲突解决验收记录](../../testing/acceptance/pr-conflict-resolution.md) | 按原文固定版本、环境和证据范围追溯 |
| [产品读取、命令入口与 Web 容器重构验收](../../testing/acceptance/product-read-command-web-refactor.md) | 按原文固定版本、环境和证据范围追溯 |
| [验收：项目远端环境适配](../../testing/acceptance/project-environment-adaptation.md) | 按原文固定版本、环境和证据范围追溯 |
| [项目更新与候选版本收敛验收](../../testing/acceptance/project-maintenance-simplification.md) | 按原文固定版本、环境和证据范围追溯 |
| [可靠性交付简化实施验收](../../testing/acceptance/reliability-simplification.md) | 按原文固定版本、环境和证据范围追溯 |
| [可靠性交付简化方案可行性审计](../../testing/acceptance/reliable-delivery-simplification-audit-20261021.md) | 按原文固定版本、环境和证据范围追溯 |
| [可靠性交付简化方案第二轮审计](../../testing/acceptance/reliable-delivery-simplification-audit-round2-20261021.md) | 按原文固定版本、环境和证据范围追溯 |
| [可靠性交付摘要验收](../../testing/acceptance/reliable-delivery-simplification.md) | 按原文固定版本、环境和证据范围追溯 |
| [Roadmap 读取、解析与转正验收计划](../../testing/acceptance/roadmap-promotion.md) | 按原文固定版本、环境和证据范围追溯 |
| [SteerLine 与 SteerRunner 职责边界审计及验收](../../testing/acceptance/runner-boundary-consolidation.md) | 按原文固定版本、环境和证据范围追溯 |
| [SteerLine / SteerRunner 协作与 Workflow 接入验收](../../testing/acceptance/runner-collaboration.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner 工作区命名前缀验收](../../testing/acceptance/runner-name-prefix.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner 短状态路径验收](../../testing/acceptance/runner-short-state-path.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner 停服最终结算](../../testing/acceptance/runner-shutdown-settlement.md) | 按原文固定版本、环境和证据范围追溯 |
| [SteerRunner 拆分后的测试精简验收](../../testing/acceptance/runner-test-convergence.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner 等待、重试状态与执行页验收规划](../../testing/acceptance/runner-wait-retry.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner 兼容接管验收](../../testing/acceptance/service-generation-recovery.md) | 按原文固定版本、环境和证据范围追溯 |
| [应用级共享 SteerRunner 服务验收](../../testing/acceptance/shared-runner-service.md) | 按原文固定版本、环境和证据范围追溯 |
| [SteerRunner 接入验收](../../testing/acceptance/steerrunner-cutover.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner generate/1 对接与最简测试管理验收](../../testing/acceptance/test-management.md) | 按原文固定版本、环境和证据范围追溯 |
| [统一启动与多项目共进程宿主验收](../../testing/acceptance/unified-startup.md) | 按原文固定版本、环境和证据范围追溯 |
| [Web 候选停止参与修复验收](../../testing/acceptance/web-candidate-stop.md) | 按原文固定版本、环境和证据范围追溯 |
| [本机 Web 前端验收](../../testing/acceptance/web-frontend.md) | 按原文固定版本、环境和证据范围追溯 |
| [工作流核验清单与收敛记录](../../testing/acceptance/workflow-verification.md) | 按原文固定版本、环境和证据范围追溯 |
| [Runner 专属验收](https://github.com/erranliu/SteerRunner/blob/codex/initialization-stop/docs/acceptance/initialization-stop.md) | Runner 原版本来源；能力与验收由其自身契约界定 |
| [Orca 远端记录](https://github.com/erranliu/SteerRunner/blob/d4969d0/docs/acceptance/orca-remote-20261017.md) | Runner 原版本来源；能力与验收由其自身契约界定 |
| [Runner 草稿 PR](https://github.com/erranliu/SteerRunner/pull/1) | Runner 原版本来源；能力与验收由其自身契约界定 |
| [application（固定迁出版本）](https://github.com/erranliu/SteerRunner/tree/5f664a9/internal/application) | Runner 原版本来源；能力与验收由其自身契约界定 |
| [execution（固定迁出版本）](https://github.com/erranliu/SteerRunner/tree/5f664a9/internal/execution) | Runner 原版本来源；能力与验收由其自身契约界定 |

## 第四批整理来源

2026-11-06 精简调用方反馈、公开会话、数据与配置、进度说明；工作区旧独立入口并入当前数据模型和 Runner 接入。旧 TUI 操作、development/v3、原生直接采集及历史 schema 均按原版本追溯，不作当前操作指引。

整理前已提交原文可用 `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:<路径>` 读取：

- `docs/product-design/decisions.md`。
- `docs/product-design/conversation-events.md`。
- `docs/product-design/worktree-preparation.md`。
- `docs/product-design/data-model.md`。
- `docs/product-design/execution-profiles.md`。
- `docs/integration/runtime-progress.md`。

下列补充来源与上方验收表合并使用；原记录的事实和结论不改写。

旧构建接管的[服务换代恢复 Spec](../../spec/service-generation-recovery.spec.md)仅作为其原版本来源；当前 v8 升级按 Runner 接入和排空规则，不从旧接管能力推定资格。

| 记录 |
| --- |
| [Spec：只保存最终回复与原会话恢复](../spec/completed/product-simplification-04-conversation-recovery.spec.md) |
| [SteerLine / SteerRunner 协作与 Workflow 接入 Plan](../../plans/runner-collaboration.plan.md) |
| [运行时测试进度 Plan](../../plans/runtime-test-progress.plan.md) |
| [Web 执行配置管理与能力探测 Plan](../../plans/web-execution-config.plan.md) |
| [SteerLine 调用方提示词收敛 Spec](../../spec/caller-prompt-convergence.spec.md) |
| [旧 Runner Check 接入退役](../../spec/legacy-environment-check-retirement.spec.md) |
| [原生异步问答最小设计](../../spec/native-async-questions.spec.md) |
| [原生执行配置边界收口 Spec](../../spec/native-execution-configuration-boundary.spec.md) |
| [Spec：Orca 普通终端完成补查询](../../spec/orca-completion-query.spec.md) |
| [Spec：项目 × 环境准备状态与统一派发门禁](../../spec/project-environment-readiness.spec.md) |
| [Spec：项目自行检测，SteerLine 调用并消费环境结果](../../spec/project-environment-workflow.spec.md) |
| [SteerLine 与 SteerRunner 职责边界收口 Spec](../../spec/runner-boundary-consolidation.spec.md) |
| [SteerLine / SteerRunner 协作、执行预设与 Workflow 接入 Spec](../../spec/runner-collaboration.spec.md) |
| [Spec：运行环境与项目运行环境最小解耦](../../spec/runtime-project-environment-decoupling.spec.md) |
| [运行时测试进度 Spec](../../spec/runtime-test-progress.spec.md) |
| [Web 执行配置管理与能力探测 Spec](../../spec/web-execution-config.spec.md) |
| [Spec：统一 Worker 的 PR 成果流程](../../spec/worker-pr-flow.spec.md) |
| [Spec：worker 停服恢复与手动继续](../../spec/worker-service-recovery.spec.md) |
| [Spec：Worker 分步执行与结构化状态推进](../../spec/worker-staged-flow.spec.md) |
| [会话与恢复精简验收](../../testing/acceptance/archive/completed/product-simplification-04-conversation-recovery.md) |
| [原生准备工具验收摘要](../../testing/acceptance/archive/completed/worker-readiness.md) |
| [验收：旧 Runner Check 接入退役](../../testing/acceptance/legacy-environment-check-retirement.md) |
| [验收：Orca 普通终端完成补查询](../../testing/acceptance/orca-completion-query.md) |
| [运行时测试进度验收](../../testing/acceptance/runtime-test-progress.md) |
| [Task18 第二轮测试进度 0/6 修复](../../testing/acceptance/test-progress-0of6-20261026.md) |
| [Web 执行配置管理与能力探测验收](../../testing/acceptance/web-execution-config.md) |
| [staged_flow.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/staged_flow.go) |

## 第五批整理来源

2026-11-06 精简本机协议、安装部署、独立调试及项目维护。当前规则分别由四个原入口维护；旧三轮环境适配/生成动作、collect 关联、inline 材料和迁移过程仅按原版本追溯，不推定当前资格。

整理前已提交原文可用 `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:<路径>` 读取：

- `docs/product-design/local-client-protocol.md`。
- `docs/integration/install-deploy.md`。
- `docs/product-design/project-maintenance.md`。
- `docs/runtime-install-debug.md`。

2026-10-15 安装迁移的项目、任务和现场目录只在原验收及固定 Git 原文保留；2026-11-01 安装清理也仅对应当次授权。项目维护 11-05/11-06 的显式准备、异步 worker 和统一就绪增量仍由各自验收保存，不能用早期人工采用/应用或后来文档整理替代验证。

以下补充来源与上方表合并使用，记录的受测构建、历史失败与未验项保持原文：

| 记录 |
| --- |
| [Runner 等待与重试 Spec](../../spec/runner-wait-retry.spec.md) |
| [统一启动 Spec](../../spec/unified-startup.spec.md) |
| [项目环境适配 Spec](../../spec/project-environment-adaptation.spec.md) |
| [2026-10-21 维护记录](../../testing/acceptance/install-shutdown-recovery-20261021.md) |
| [v8 安装清理记录](../../testing/acceptance/runner-v8-install-reset.md) |
| [项目统一就绪验收](../../testing/acceptance/project-readiness.md) |
| [显式材料准备验收](../../testing/acceptance/project-maintenance-preparation.md) |
| [项目维护收敛 Spec](../../spec/project-maintenance-simplification.spec.md) |
| [项目维护收敛 Plan](../../plans/project-maintenance-simplification.plan.md) |
| [原项目维护验收](../../testing/acceptance/project-maintenance.md) |
| [原 Codex 沙箱与 Git 验收](../../testing/acceptance/codex-sandbox-git.md) |

## 首轮文档合并

2026-11-06 按阅读目的将六份正文合为三份，docs/ 的 Markdown 数量由 25 减至 22。下表旧路径不再保留跳转文件；已提交原文仍可用 `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:<旧路径>` 读取。前六批修正的当前规则由新入口承接，历史受测构建与结论保持原记录。

| 删除的旧路径 | 当前入口 |
| --- | --- |
| `docs/product-design/activity.md`、`docs/product-design/conversation-events.md` | [执行观察与结果](../../../docs/product-design/execution-observability.md)：公开活动、最近消息、final 与正文来源分别判断 |
| `docs/product-design/execution-profiles.md`、`docs/integration/developer-config.md` | [执行配置](../../../docs/product-design/execution-configuration.md)：预设、冻结、生效、开发覆盖与发布限制 |
| `docs/product-design/project-management.md`、`docs/product-design/project-structure.md` | [项目接入与管理](../../../docs/product-design/project-onboarding.md)：当前切换与拟议添加/PS 核验/登记分章 |

本次移除重复说明，不改变已有准入、事务、预算、身份、恢复或授权规则。活动与会话沿[第四批来源](#第四批整理来源)和[第六批记录](docs-sixth-batch-review.md)追溯；配置历史见第四批及[开发者配置验收](../../testing/acceptance/developer-configuration.md)，项目添加的拟议范围与原调查见[第三批来源](README.md#第三批整理来源)。六份原文引用的有效契约、源码及证据入口随新正文保留。

准备提示词仅调整外围导航，复制正文保持 **7,613 bytes**、69 个 CRLF，SHA-256 为 `e18d16c36a050a1fac8d4dcb11af89e765b9dd2e7144ccbc8f8d65252bdba2fc`，与已审核的第六批逐字节一致。本次没有新的提示词正文差异、生产接线、正式协议/writer、黄金样本或 CI 修改；另仅更新一处 Go 注释中的文档路径。

本次只做文档静态验证，不执行模型、真实 PR、服务启动或安装维护；合并不使拟议添加与 PS 核验器获得实现状态，不扩大原平台验收或远端保护结论。

静态检查通过：703 份 Markdown、4,364 条本地链接无缺失，50 条相关锚点无误，22 份 docs 文档均被索引直接覆盖；编号与 diff --check 通过。六份原文的 36 个不同引用目标（合并内部入口去重后）保留，28 份其他既有文档只改导航目标。复制正文、原型字节及生产资产核对一致，三份新正文由合并前 31,626 bytes 减至 26,530 bytes。

PR 提交前同步 main 的异步活动中心、测试生成需处理提示和显式放弃 schema 40 旧运行修复：保留新增设计与只读投影规则，当前 schema 定位更新为 41，明确普通运行与放弃归档的不同格式范围。主干新增 async-activities.md 后 docs 为 23 份，全部索引覆盖；最终检查为 710 份 Markdown、4,398 条本地链接及 68 条相关锚点无误，编号/差异检查和提示词包测试通过。原复制正文仍为已审核的 7,613 bytes，运行代码相对 main 仅一处文档路径注释；原型迁移的 Git 正文一致。
