# 验收归档

归档日期：2026-10-07。已结算的历史记录放 completed/，被放弃或退役的旧方案放 superseded/。当前计划与未闭合项见[验收入口](../README.md)。

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
| #44 WinUI 基座，后续验收要求撤销，代码删除待 #91 | [历史交付/原未验](superseded/phase2-winui-foundation.md) |
| #46 提案/草稿，退役 | [旧规划](superseded/phase2-planning-drafts.md) |
| #47 规划会话，退役 | [旧规划](superseded/phase2-planning-sessions.md) |
| #48 文档发布/创建，退役 | [旧规划](superseded/phase2-document-publish.md) |
| #49 DAG 页面，退役 | [旧规划](superseded/phase2-roadmap-ui.md) |
| #50 执行页面，退役 | [旧规划](superseded/phase2-execution-ui.md) |
| #51 规划对话，退役 | [旧规划](superseded/phase2-conversation-ui.md) |
| #52 审阅/确认，退役 | [旧规划](superseded/phase2-review-ui.md) |
| 旧三节点，退役 | [2.1](superseded/phase2-execution.md) · [2.2](superseded/phase2-planning.md) · [2.3](superseded/phase2-main-agent.md) |

2026-10-13 用户清理旧规划；上述记录及旧三节点/#44/#49/#50只作追溯，不进入当前队列，不自动恢复或复用编号。当前仅[任务 #91](../../spec/tui-http-client.spec.md)，#44 实现删除待实施，原未验不标通过。
