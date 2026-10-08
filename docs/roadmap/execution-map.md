# 正式执行 Roadmap

开发执行方式：roadmap-runner 直接使用 Codex；本表是用户确认的开发任务与依赖权威。

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | 执行中 | [Spec](../spec/local-runtime-isolation.spec.md) · [Plan](../spec/local-runtime-isolation.plan.md) | PR [#247](https://github.com/erranliu/SteerLine/pull/247)；候选 `ad22d476` 已补验 LI3-A state/UI alias 与 LI2-A 真实 batch/console/service，自动验收通过，等待新独立复审；[记录](../acceptance/local-runtime-isolation.md) |
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

2026-10-08，独立复审 round 1 在 [PR #247](https://github.com/erranliu/SteerLine/pull/247#issuecomment-6058531009) 发现 P1/P2；P1 局部修复提交 `74ea6413214d980bd0bb1052c8b3ed5d8811669d`，原执行者继续补 P2，最终复验待做。详见[验收记录](../acceptance/local-runtime-isolation.md)。

2026-10-08，候选 `066a5985677cccb4fdb4650bf669ddf15050de90`（tree `98856831907b85a37ba8b5f3364726b0659de358`，Windows/amd64 clean tree）的原 runtime runner、P1 子集、L1、`delivery.basic` 与 Linux/amd64 编译结果保留各自已测范围；round 2 后确认 LI3-A 因缺省 state/UI junction 实际写入安装侧而失败，LI2-A 因未执行 generated batch/service 入口而待验。见[任务验收记录](../acceptance/local-runtime-isolation.md#2026-10-08-独立复审-round-2-缺省写路径失败及-batch-未验)。PR #247 未合并；#83 保持执行中。

2026-10-08，修复候选 `ad22d476f87e019a9ad5d0118758b253dd091094`（tree `e2470c18a0d29979006aa5e12a440f2035bf5fa0`，Windows/amd64 clean source）通过 LI1-A–LI4-A：state/UI junction 的脚本与 native 入口在副作用前拒绝；`Operation service`、直接 generated `service-window.bat` 和 generated `console.bat` 均真实运行并记录进程/路径/实例/停服回执；P1 10 项、L1、delivery.basic、owner/boundary 定向、ConPTY smoke 和 Linux/amd64 编译通过。证据见[最终验收记录](../acceptance/local-runtime-isolation.md)。PR #247 未合并；#83 保持执行中并等待新独立复审。

流程规则见[RoadmapGuide](RoadmapGuide.md)；[二期节点验收](../acceptance/phase2.md)目前仅规划。
