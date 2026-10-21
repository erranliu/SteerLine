# 验收归档

最近整理：2026-10-21；早期归档自 2026-10-07 起。已结算的历史记录放 completed/，被放弃或退役的旧方案放 superseded/。当前计划与未闭合项见[验收入口](../README.md)。

归档保留原源码、范围、失败、skip和限制，不代表当前构建通过；旧方案归档不代表其任务已完成。固定Git契约仍按原commit/路径读取。JSON/JSONL保持原字节，只修复Markdown导航。

## 历史结果

| 范围 | 记录 |
| --- | --- |
| #288 后全项目回归、过期测试修复与原生审核拒绝 | [结果](completed/all-tests-after-288-2026-10-10.md) · [索引](completed/all-tests-after-288-2026-10-10.json) |
| #266 后全项目回归与分步反馈修复 | [结果](completed/all-tests-after-266-2026-10-10.md) · [索引](completed/all-tests-after-266-2026-10-10.json) |
| #243 后全项目回归与修复 | [结果](completed/all-tests-after-243-2026-10-08.md) · [索引](completed/all-tests-after-243-2026-10-08.json) |
| 全项目历史基线 | [基线摘要](completed/latest-baseline.md) · [结果索引](completed/baseline-results.json) |
| #39 双本机候选 | [验收摘要](completed/candidate-acceptance.md) |
| #38 CC5人工确认 | [原确认](completed/manual-acceptance.md) |
| 三窗及内嵌反馈 | [控制台验收](completed/product-console.md) |
| PR #234控制台体验 | [审计与验证](completed/console-primary-workspace.md) |
| L1精简与并行 | [验证](completed/l1-optimization.md) · [结果索引](completed/l1-optimization-results.json) |
| PR #236固定原生配置 | [记录与精选证据](completed/product-simplification-02-configuration/README.md) |
| PR #238资源保留与CLI收口 | [记录与精选证据](completed/product-simplification-03-resources-cli/README.md) |
| #238后完整回归 | [结果](completed/all-tests-after-238-2026-10-07.md) · [索引](completed/all-tests-after-238-2026-10-07.json) |
| 原生准备工具 | [原结果](completed/worker-readiness.md) |
| 历史文档/规划检查 | [检查记录](completed/roadmap-planning.md) |
| 验证目录清理与更早证据 | [历史取证](completed/history.md#历史取证) |

## 旧二期与延期方案（已退出当前规划）

| 原归属 | 参考记录 |
| --- | --- |
| #40 固定版本与双入口，退出当前规划 | [规划](superseded/versioned-runtime.md) |
| #41 dev隔离与stable提升，退出当前规划 | [规划](superseded/runtime-isolation-promotion.md) |
| #43 独立主agent探测，不再安排 | [旧规划](superseded/phase2-agent-capabilities.md) |
| #44 WinUI 基座，后续验收要求撤销，代码已由 #91 删除 | [历史交付/原未验](superseded/phase2-winui-foundation.md) |
| #46 提案/草稿，退役 | [旧规划](superseded/phase2-planning-drafts.md) |
| #47 规划会话，退役 | [旧规划](superseded/phase2-planning-sessions.md) |
| #48 文档发布/创建，退役 | [旧规划](superseded/phase2-document-publish.md) |
| #49 DAG 页面，退役 | [旧规划](superseded/phase2-roadmap-ui.md) |
| #50 执行页面，退役 | [旧规划](superseded/phase2-execution-ui.md) |
| #51 规划对话，退役 | [旧规划](superseded/phase2-conversation-ui.md) |
| #52 审阅/确认，退役 | [旧规划](superseded/phase2-review-ui.md) |
| 旧三节点，退役 | [2.1](superseded/phase2-execution.md) · [2.2](superseded/phase2-planning.md) · [2.3](superseded/phase2-main-agent.md) |

2026-10-13 用户清理旧规划；上述记录及旧三节点/#44/#49/#50只作追溯，不进入当前队列，不自动恢复或复用编号。[任务 #91](../../spec/tui-http-client.spec.md)已交付并删除 #44，TUI 随后退役；当前执行见[正式清单](../../roadmap/execution-map.md)，原未验不标通过。

## 2026-10-21 补充历史结果

| 原活动 | 记录与范围 |
| --- | --- |
| Worker 三层重构 | [验收](completed/worker-three-layer.md)：TL-A01–TL-A07 自动通过，PR #313；执行随后迁至 Runner |
| 第四批产品精简 | [验收](completed/product-simplification-04-conversation-recovery.md)：原最小自动验证与最终基线，PR #243 |
| 2026-10-14/15 完整回归 | [记录](completed/full-regression-2026-10-14.md) · [原结果索引](completed/full-regression-2026-10-14.json)：PR #315，原失败/skip 与通过范围保留 |
| 2026-10-15 SQLite 取消修复后 Full | [记录](completed/sqlite-cancellation-full-2026-10-15.md)：仅证明原 `beec0e35`，不代表当前构建重跑 |

## 退役 Orca 方案与原版本证据

- [本机原生通用 worker](superseded/orca-native-worker.md)、[本机原生编排及 ON 未验项](superseded/worker-flow-orca-native.md)：旧 worker-start/Dispatch 已删除。
- [supervised 双目标](superseded/orca-supervised-targets.md)、[分步结果](superseded/orca-supervised-results.md)、[原生结果探针](superseded/orca-supervised-result-probe-2026-10-10.md)：旧 supervised 路线退役，探针不作为单终端或 Runner 交付证据。
- [本机输入与就绪](superseded/orca-local-input-readiness.md)：本机 Orca 已放弃；原有限样本通过范围保留，不推定远端通过。

对应[退役 Spec/Plan](../../archive/spec/retired/README.md#2026-10-21-orca-历史方案归档)与[迁出前架构说明](../../archive/design/orca-native-worker.md)保留追溯。所有旧失败、取消和未验均保留原结果；当前接入及验证应按[Runner 接入](../../integration/steerrunner.md)重核。
