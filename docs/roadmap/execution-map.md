# 正式执行 Roadmap

开发执行方式：roadmap-runner 直接使用 Codex；本表是用户确认的开发任务与依赖权威。

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | 待复审/合并 | [Spec](../spec/local-runtime-isolation.spec.md) · [Plan](../spec/local-runtime-isolation.plan.md) | PR [#247](https://github.com/erranliu/SteerLine/pull/247)；LI1-A–LI4-A 已通过；无本任务人工待验；[记录](../acceptance/local-runtime-isolation.md) |
| 45 | 本机客户端协议与历史查询 | 无 | 待执行 | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) | 未验；[记录](../acceptance/phase2-local-api.md) |
| 44 | WinUI 基座与原生输入 | #45 | 等待 #45 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) | 未验；[记录](../acceptance/phase2-winui-foundation.md)，人工项最迟2.1签收 |

## 推进顺序

1. 按 #83 → #45 → #44 串行推进；启动时固定 Spec/Plan commit 和路径，并核对最新默认主干。
2. #83与#45无新增硬依赖，共享宿主接线串行合入；#45经复审、必要验证和真实合并，结果同步且成果进入最新主干后启动#44。
3. 后续界面任务留在[草稿](drafts/roadmap.md)，原型确认后再纳入。

## 待人工验收

| 范围 | 状态 / 安排 | 明细 |
| --- | --- | --- |
| 一期单安装版签收 | 实现收尾#83已排入；交付并满足准备条件后组织人工签收 | [一期活动](../acceptance/phase1.md) |
| #44原生输入与可达性 | #44已排入；交付后验UI2-M/UI4-M，最迟2.1签收 | [记录](../acceptance/phase2-winui-foundation.md) |

自动补验：[Codex race](../acceptance/local-codex-app-server.md) · [Orca原生编排](../acceptance/worker-flow-orca-native.md) · [Orca安全重建](../acceptance/worker-service-recovery.md)。

## 执行记录

2026-10-08，用户确认采用 Codex 直接开发流程；#83/#45待执行，#44等待#45，当前未启动、未验。外部中心配置问题不作为本流程前置；具体产品测试的环境要求仍按 Spec/Plan 核对。历史查询与本次流程调整见[记录](../acceptance/roadmap-admission.md)。

2026-10-08，#83 实现与 LI1-A–LI4-A 自动验收通过；[PR #247](https://github.com/erranliu/SteerLine/pull/247) 待独立复审/合并。实际命令与临时证据见[验收记录](../acceptance/local-runtime-isolation.md)。

流程规则见[RoadmapGuide](RoadmapGuide.md)；[二期节点验收](../acceptance/phase2.md)目前仅规划。
