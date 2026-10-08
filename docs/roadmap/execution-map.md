# 正式执行 Roadmap

开发执行方式：roadmap-runner 直接使用 Codex；本表是用户确认的开发任务与依赖权威。

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | 执行中 | [Spec](../spec/local-runtime-isolation.spec.md) · [Plan](../spec/local-runtime-isolation.plan.md) | PR [#247](https://github.com/erranliu/SteerLine/pull/247)；round 4 SQLite 共享硬链接 P1 已局部修复；干净候选 `92eaf9b8` 的 LI1-A–LI4-A 完整 runtime/六条真实 DB/WAL/SHM 负例通过并清理；生产实现 `9aeed603` 定向/L1/Linux 编译通过，精确映射见记录；等待新独立复审，PR 未合并，不释放后继；[记录](../acceptance/local-runtime-isolation.md) |
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

2026-10-08，round 3 独立复审候选 `f5c729f985f56f1a59c7506a6bc07dfc6e9ab402` 确认 direct build.ps1 漏 canonical containment（LI2-A 失败）及 stale instance/config 请求计数错误（LI3-A 该子项未有效验证）；撤销此前全部通过汇总，历史原始证据保留。独立局部 fixer 仅修 runtime 请求观察及真实正常发布的计数增长断言；一次受影响 controlled 场景通过并清理完成（HEAD f5c729f 加局部测试/结果文档修改、source_dirty=true，测试 blob 072258ca）；两条拒绝 0→0，真实正常发布 0→2。P1 留给原执行者，最终候选及新独立复审仍待完成。PR #247 未合并、不释放后继；[明细](../acceptance/local-runtime-isolation.md#2026-10-08-独立复审-round-3-请求观察局部修复)。

2026-10-09，原执行者候选 `f838dcf587efb2cb22992e6354640f530e0df8b7`（tree `deb3ae0f1dbc3b2a9402e0ecc0681a2a5beb4682`，Windows/amd64 clean tree）修复 LI2-A direct `build.ps1` canonical containment 和 LI3-A 正式 request 计数断言；同一受控 L3 runtime 通过，1 test/0 skip/清理完成，P1 10 项、L1（3 workers/12 groups/426 tests/0 skip）、`delivery.basic` 与 Linux/amd64 编译通过。LI3-A stale instance/config 两条拒绝均 0→0，正常产品领取正式 request 0→2。LI2-A 源码 bin junction 与输出 hardlink 均拒绝，普通构建/重建成功，安装 exe/DB/task facts/bin/service instance 保持。前序失败运行的 Windows scratch 路径、Git line-ending 环境和 CLI dirty 字段断言原因、产物定位见[验收记录](../acceptance/local-runtime-isolation.md)。PR #247 仍 OPEN、未合并；#83 保持执行中并等待新的独立复审，不释放后继。

流程规则见[RoadmapGuide](RoadmapGuide.md)；[二期节点验收](../acceptance/phase2.md)目前仅规划。

2026-10-09，round 4 受审 HEAD `054decf` 的 SQLite SHM 共享硬链接真实负例失败，LI3-A 整体已验汇总撤销；临时 installed 哨兵变为 32768 字节，产品 CLI/wrapper 最终 invalid_batch 不构成副作用前拒绝。当时局部修复与精确候选复验待完成；[明细](../acceptance/local-runtime-isolation.md#2026-10-09-round-4-独立复审sqlite-共享硬链接失败)。PR #247 未合并、不释放后继。

2026-10-09，round 4 局部修复生产 commit `9aeed603d46bf7d566b2daf183855dc3c8316fc4` 拒绝 SQLite DB/WAL/SHM/journal 共享硬链接，Windows 真实 link count 在 app/host/reader/writer SQLite 打开前核验。最终干净候选 `92eaf9b8163f71be0554265ddba85ee0708855ee`（仅场景时点调整，tree `28f8395f90b71d3f8a7d0dc139c43d17dbbd2345`）完整 L3 runtime 1 test/0 skip/262.63 秒通过并清理；六条产品 CLI/debug wrapper 硬链接负例在副作用前明确拒绝，哨兵与全部 SQLite 快照逐次不变，普通单链接仍可用。`9aeed603` 定向 9 个顶层测试、L1 12 groups/426 tests/0 skip、Linux/amd64 编译通过；之后生产 Go 无变化，回归精确映射复用；旧 `f838dcf` delivery 只保留原范围。首轮 runtime 的运行中 installed DB 字节比较失败及清理证据保留，新的独立目录复验通过。LI3-A 已按新证据修正为已验收；[明细](../acceptance/local-runtime-isolation.md#2026-10-09-round-4-局部修复与精确候选复验)。PR #247 仍未合并，#83 执行中并等待新独立复审，不释放后继。
