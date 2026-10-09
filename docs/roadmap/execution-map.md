# 正式执行 Roadmap

开发执行方式：roadmap-runner 直接使用 Codex；本表是用户确认的开发任务与依赖权威。

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | 实现完成 | [Spec](../spec/local-runtime-isolation.spec.md) · [Plan](../spec/local-runtime-isolation.plan.md) | [PR #247](https://github.com/erranliu/SteerLine/pull/247) 已真实合并，merge `f7c6f1d0`；第五轮独立复审无新问题；LI1-A–LI4-A 自动验收通过，无本任务人工项；[记录及验证范围](../acceptance/local-runtime-isolation.md) |
| 84 | Worker 分步执行与结构化状态推进 | 无；复用已合入 #83 的隔离基线 | 执行中 | [Spec](../spec/worker-staged-flow.spec.md) · [Plan](../spec/worker-staged-flow.plan.md) | 产品自动 WFS-A1–A4 未验；WFS-M1 可后置至一期组合验收；[专属计划与记录](../acceptance/worker-staged-flow.md) |
| 45 | 本机客户端协议与历史查询 | 无 | 执行中；PR #249 修复 / 待新独立复审 | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) | [PR #249](https://github.com/erranliu/SteerLine/pull/249) 在途，API2 历史失败未闭合；[记录](../acceptance/phase2-local-api.md) |
| 44 | WinUI 基座与原生输入 | #45 | 等待 #45 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) | 未验；[记录](../acceptance/phase2-winui-foundation.md)，人工项最迟2.1签收 |

## 推进顺序

1. #83 已合入；最新用户授权 #84 与 #45 独立并行实施，共享接线按 #84 → #45 集成，#44 继续等待 #45。启动时使用已提交且执行工作区可读取的 Spec/Plan commit 和路径，并核对最新默认主干；本次纳入清单不等于已经启动。
2. #84 与 #45 无新增业务硬依赖，共同状态/存储/宿主/客户端接线由 #84 负责语义并按 #84 后 #45 集成，#45 复核 #84 的实际字段与接口版本。#44 仍以 #45 为硬前置；#45 经复审、必要验证和真实合并，结果同步且成果进入最新主干后启动 #44。已有在途执行先核对原 run 与固定契约，不因清单编辑自动中断或切换规格。
3. 后续界面任务留在[草稿](drafts/roadmap.md)，原型确认后再纳入。

## 待人工验收

| 范围 | 状态 / 安排 | 明细 |
| --- | --- | --- |
| 一期单安装版签收 | 隔离能力 #83 已合入；待 #84 交付及最小自动验证、固定新安装构建并满足原生准备条件后，组织组合/24小时/同构建恢复人工签收；窗口待安排 | [一期活动](../acceptance/phase1.md) |
| #84 步骤辨识与反馈/合并 | WFS-M1 待验；本任务自动能力交付后，可与一期 P1A1-M 同一现场集中验收，分别登记结果；不阻塞无关二期研发 | [专属记录](../acceptance/worker-staged-flow.md) |
| #44原生输入与可达性 | #44已排入；交付后验UI2-M/UI4-M，最迟2.1签收 | [记录](../acceptance/phase2-winui-foundation.md) |

自动补验：[Codex race](../acceptance/local-codex-app-server.md) · [Orca原生编排](../acceptance/worker-flow-orca-native.md) · [Orca安全重建](../acceptance/worker-service-recovery.md)。

## 执行记录

2026-10-09 #84 集成主干原生 Orca 改动（PR #259，主干 ade7042b），沿原固定 c4bcbec Spec/Plan 实现通用步骤，未恢复退役 launcher。用户明确调整 Orca usage 验收为公开接口不可用如实登记，真实三轮/原身份/PR/清理仍必验；[专属记录](../acceptance/worker-staged-flow.md)同步原失败与新验证范围。历史预留 #86 保留身份但不进入 DAG，主干直接实施结果见[原生 Orca 调整](../architecture/orca-native-worker.md)，不作为 #84 等待边。


2026-10-09 最新授权：“rebase，#84任务可以并行执行了, #84用 GPT6.1 Sol High”。#84 开发聊天使用 gpt-6.1-sol/high（产品 E2E 仍按固定 Plan 的原模型/原生权限），与 #45 独立并行；共享主干接线先 #84 后 #45，#44 硬前置保持。#84 固定 Spec/Plan commit `c4bcbec06289c9f5b086c8f1f98859a0f858d7e5`，代码起点同 SHA，工作树 b123、分支 codex/task-84-worker-staged-flow；实现/验证进行中，尚未独立复审或合并。#45 原执行者仍处理 PR #249，API2 失败未闭合，不由本任务接管。此前串行纳入记录是历史安排，不覆盖本段最新授权。


2026-10-09，用户确认将 #84 纳入 execution-map，沿用已预留身份及独立 Spec/Plan/专属验收。状态为待执行、尚未启动，产品自动与人工项仍未验；#83 已合入，无未完成硬前置。按一期收尾安排，剩余串行顺序为 #84→#45→#44；#45→#44 的硬依赖保持，共享接线排期不新增业务依赖。文档版本在实际启动前提交并固定，不创建外部 SteerLine Task/DAG。

2026-10-08，用户确认采用 Codex 直接开发流程；#83/#45待执行，#44等待#45，当前未启动、未验。外部中心配置问题不作为本流程前置；具体产品测试的环境要求仍按 Spec/Plan 核对。历史查询与本次流程调整见[记录](../acceptance/roadmap-admission.md)。

2026-10-08，独立复审 round 1 在 [PR #247](https://github.com/erranliu/SteerLine/pull/247#issuecomment-6058531009) 发现 P1/P2；P1 局部修复提交 `74ea6413214d980bd0bb1052c8b3ed5d8811669d`，原执行者继续补 P2，最终复验待做。详见[验收记录](../acceptance/local-runtime-isolation.md)。

2026-10-08，候选 `066a5985677cccb4fdb4650bf669ddf15050de90`（tree `98856831907b85a37ba8b5f3364726b0659de358`，Windows/amd64 clean tree）的原 runtime runner、P1 子集、L1、`delivery.basic` 与 Linux/amd64 编译结果保留各自已测范围；round 2 后确认 LI3-A 因缺省 state/UI junction 实际写入安装侧而失败，LI2-A 因未执行 generated batch/service 入口而待验。见[任务验收记录](../acceptance/local-runtime-isolation.md#2026-10-08-独立复审-round-2-缺省写路径失败及-batch-未验)。PR #247 未合并；#83 保持执行中。

2026-10-08，修复候选 `ad22d476f87e019a9ad5d0118758b253dd091094`（tree `e2470c18a0d29979006aa5e12a440f2035bf5fa0`，Windows/amd64 clean source）通过 LI1-A–LI4-A：state/UI junction 的脚本与 native 入口在副作用前拒绝；`Operation service`、直接 generated `service-window.bat` 和 generated `console.bat` 均真实运行并记录进程/路径/实例/停服回执；P1 10 项、L1、delivery.basic、owner/boundary 定向、ConPTY smoke 和 Linux/amd64 编译通过。证据见[最终验收记录](../acceptance/local-runtime-isolation.md)。PR #247 未合并；#83 保持执行中并等待新独立复审。

2026-10-08，round 3 独立复审候选 `f5c729f985f56f1a59c7506a6bc07dfc6e9ab402` 确认 direct build.ps1 漏 canonical containment（LI2-A 失败）及 stale instance/config 请求计数错误（LI3-A 该子项未有效验证）；撤销此前全部通过汇总，历史原始证据保留。独立局部 fixer 仅修 runtime 请求观察及真实正常发布的计数增长断言；一次受影响 controlled 场景通过并清理完成（HEAD f5c729f 加局部测试/结果文档修改、source_dirty=true，测试 blob 072258ca）；两条拒绝 0→0，真实正常发布 0→2。P1 留给原执行者，最终候选及新独立复审仍待完成。PR #247 未合并、不释放后继；[明细](../acceptance/local-runtime-isolation.md#2026-10-08-独立复审-round-3-请求观察局部修复)。

2026-10-09，原执行者候选 `f838dcf587efb2cb22992e6354640f530e0df8b7`（tree `deb3ae0f1dbc3b2a9402e0ecc0681a2a5beb4682`，Windows/amd64 clean tree）修复 LI2-A direct `build.ps1` canonical containment 和 LI3-A 正式 request 计数断言；同一受控 L3 runtime 通过，1 test/0 skip/清理完成，P1 10 项、L1（3 workers/12 groups/426 tests/0 skip）、`delivery.basic` 与 Linux/amd64 编译通过。LI3-A stale instance/config 两条拒绝均 0→0，正常产品领取正式 request 0→2。LI2-A 源码 bin junction 与输出 hardlink 均拒绝，普通构建/重建成功，安装 exe/DB/task facts/bin/service instance 保持。前序失败运行的 Windows scratch 路径、Git line-ending 环境和 CLI dirty 字段断言原因、产物定位见[验收记录](../acceptance/local-runtime-isolation.md)。PR #247 仍 OPEN、未合并；#83 保持执行中并等待新的独立复审，不释放后继。

流程规则见[RoadmapGuide](RoadmapGuide.md)；[二期节点验收](../acceptance/phase2.md)目前仅规划。

2026-10-09，round 4 受审 HEAD `054decf` 的 SQLite SHM 共享硬链接真实负例失败，LI3-A 整体已验汇总撤销；临时 installed 哨兵变为 32768 字节，产品 CLI/wrapper 最终 invalid_batch 不构成副作用前拒绝。当时局部修复与精确候选复验待完成；[明细](../acceptance/local-runtime-isolation.md#2026-10-09-round-4-独立复审sqlite-共享硬链接失败)。PR #247 未合并、不释放后继。

2026-10-09，round 4 局部修复生产 commit `9aeed603d46bf7d566b2daf183855dc3c8316fc4` 拒绝 SQLite DB/WAL/SHM/journal 共享硬链接，Windows 真实 link count 在 app/host/reader/writer SQLite 打开前核验。最终干净候选 `92eaf9b8163f71be0554265ddba85ee0708855ee`（仅场景时点调整，tree `28f8395f90b71d3f8a7d0dc139c43d17dbbd2345`）完整 L3 runtime 1 test/0 skip/262.63 秒通过并清理；六条产品 CLI/debug wrapper 硬链接负例在副作用前明确拒绝，哨兵与全部 SQLite 快照逐次不变，普通单链接仍可用。`9aeed603` 定向 9 个顶层测试、L1 12 groups/426 tests/0 skip、Linux/amd64 编译通过；之后生产 Go 无变化，回归精确映射复用；旧 `f838dcf` delivery 只保留原范围。首轮 runtime 的运行中 installed DB 字节比较失败及清理证据保留，新的独立目录复验通过。LI3-A 已按新证据修正为已验收；[明细](../acceptance/local-runtime-isolation.md#2026-10-09-round-4-局部修复与精确候选复验)。PR #247 仍未合并，#83 执行中并等待新独立复审，不释放后继。

2026-10-09，第五轮独立 reviewer `/root/review83_round5` 未发现新产品问题或固定需求遗漏，协调核对候选、实际生产路径和精确验证映射并在 [PR 评论](https://github.com/erranliu/SteerLine/pull/247#issuecomment-6065781425)登记。GitHub 确认 [PR #247](https://github.com/erranliu/SteerLine/pull/247) 在北京时间 2026-10-09 01:50:18 合并，审核 head `2ce81db45d7bff32a1ffb6134210995529f01b84`，merge commit `f7c6f1d05c78172fe8133f7c1763718df0165fe1`，成果已进入默认主干。#83 实现完成；自动验收 LI1-A–LI4-A 已通过，无任务人工项。一期组合/24小时/同构建恢复签收仍待安排，未宣称完成。可选文件符号链接负例未运行，复审临时探针清理被自动审批拒绝而残留三个合成文件的事实分别见[验收明细](../acceptance/local-runtime-isolation.md#2026-10-09-交付结算与独立复审)。本结算记录进入主干后按原序启动#45；不提前启动等待#45的#44，也不扩大到草稿任务。
