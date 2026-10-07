# 正式执行 Roadmap

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | 待中心准入 | [Spec](../spec/local-runtime-isolation.spec.md) · [Plan](../spec/local-runtime-isolation.plan.md) | 未验；[记录](../acceptance/local-runtime-isolation.md) |
| 45 | 本机客户端协议与历史查询 | 无 | 待中心准入 | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) | 未验；[记录](../acceptance/phase2-local-api.md) |
| 44 | WinUI 基座与原生输入 | #45 | 待中心准入 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) | 未验；[记录](../acceptance/phase2-winui-foundation.md)，人工项最迟2.1签收 |

## 推进顺序

1. 固定上述三项文档版本，办理中心准入并核对回执；当前暂不派发。
2. #83与#45无新增硬依赖，共享宿主接线串行合入；#45交付后推进#44。
3. 后续界面任务留在[草稿](drafts/roadmap.md)，原型确认后再纳入。

## 待人工验收

| 范围 | 状态 / 安排 | 明细 |
| --- | --- | --- |
| 一期单安装版签收 | 实现收尾#83已排入；交付并满足准备条件后组织人工签收 | [一期活动](../acceptance/phase1.md) |
| #44原生输入与可达性 | #44已排入；交付后验UI2-M/UI4-M，最迟2.1签收 | [记录](../acceptance/phase2-winui-foundation.md) |

自动补验：[Codex race](../acceptance/local-codex-app-server.md) · [Orca原生编排](../acceptance/worker-flow-orca-native.md) · [Orca安全重建](../acceptance/worker-service-recovery.md)。

## 同步状态

最近中心查询：2026-10-07，配置校验失败。固定commit/摘要、Task/DAG版本及创建回执待核对；详见[准入记录](../acceptance/roadmap-admission.md)。

流程规则见[RoadmapGuide](RoadmapGuide.md)；[二期节点验收](../acceptance/phase2.md)目前仅规划。
