# 正式执行 Roadmap

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../Roadmap/execution-map.md)。

开发执行方式：roadmap-runner 直接使用 Codex；本表是用户确认的开发任务与依赖权威。

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | 实现完成 | [Spec](../../spec/local-runtime-isolation.spec.md) · [Plan](../../plans/local-runtime-isolation.plan.md) | [PR #247](https://github.com/erranliu/SteerLine/pull/247) 已真实合并，merge `f7c6f1d0`；第五轮独立复审无新问题；LI1-A–LI4-A 自动验收通过，无本任务人工项；[记录及验证范围](../../testing/acceptance/local-runtime-isolation.md) |
| 84 | Worker 分步执行与结构化状态推进 | 无；复用已合入 #83 的隔离基线 | 实现完成 | [Spec](../../spec/worker-staged-flow.spec.md) · [Plan](../../plans/worker-staged-flow.plan.md) | [PR #262](https://github.com/erranliu/SteerLine/pull/262) 已真实合并，merge `267858f0`；2026-10-10 合入后独立复审无已确认缺陷，34 项定向检查通过。WFS-A1–A4 按历史精确来源与已授权 Orca usage 条件结算；WFS-M1 待人工验收。后续 supervised 规范 final 缺口保留历史；当前单终端本机有限样本及本机最终 draft 保护未跑真机、远端已准备环境有限样本通过与首次信任阻塞后原任务自动继续未通过见[最新记录](../../testing/acceptance/orca-terminal-remote-retest-2026-10-10.md)，旧证据不覆盖当前全路线；[记录、待验及当前限制](../../testing/acceptance/worker-staged-flow.md#2026-10-10-真实合并与合入后独立复审) |
| 45 | 本机客户端协议与历史查询 | 无 | 实现完成 | [Spec](../../spec/phase2-local-api.spec.md) · [Plan](../../plans/phase2-local-api.plan.md) | [PR #249](https://github.com/erranliu/SteerLine/pull/249) 已真实合并，head `73ef956f`、merge `4786934a`；第五轮独立终审无新确认缺陷。当前映射 L1 479 项、HTTP 18 项、服务 8 项及反馈组合通过，独立 actor/stop/stage 补验通过。API2 历史 SHM 自动失败保留，仅该根因/瞬态可用性风险按真人原授权登记 `accepted_exception`；原结算时点的 supervised final 缺口保留；当前单终端能力与待验见[最新记录](../../testing/acceptance/orca-terminal-remote-retest-2026-10-10.md)，不在 SHM 例外范围；[结算、证据与限制](../../testing/acceptance/phase2-local-api.md#2026-10-10-第五轮独立终审与真实合并结算) |
| 44 | WinUI 基座与原生输入 | #45（已交付） | 实现完成 | [Spec](../spec/retired/phase2-winui-foundation.spec.md) · [Plan](../spec/retired/phase2-winui-foundation.plan.md) | [PR #273](https://github.com/erranliu/SteerLine/pull/273) 已真实合并，head `b3b52c24`、merge `f04fdf08`；独立复审发现关闭草稿竞态，局部修复后新终审无阻塞缺陷。当前映射 19 tests/0 fail/0 skip、WinUI build 通过；旧 c5c9 真实运行证据保留来源。UI2-M/UI4-M/UI5-M 待人工，最迟 2.1 签收；[结算、证据与限制](../../testing/acceptance/archive/superseded/phase2-winui-foundation.md#2026-10-10-真实合并与交付结算) |
| 87 | Worker 停机对账与恢复一致性修复 | 无未交付业务硬前置；消费当前已合入 worker 基线 | 实现完成 | 用户授权本聊天直接执行，不经 roadmap-runner；[Spec](../../spec/worker-stop-recovery.spec.md) · [Plan](../../plans/worker-stop-recovery.plan.md) | [PR #260](https://github.com/erranliu/SteerLine/pull/260) 已真实合并，merge `d951ca87`；独立复审 clean，WSR-A1–A8 自动通过（含双真实路径），无任务人工项；[专属记录](../../testing/acceptance/worker-stop-recovery.md) |

## 推进顺序

1. 本轮用户授权 #83/#84/#45/#44 均已实现、独立核验并真实合入。已合入任务不重新派发；SHM 限定风险接受、历史失败及人工未验事实保留。后续任务须另经用户确认。
2. #84 与 #45 无新增业务硬依赖，共同状态/存储/宿主/客户端接线由 #84 负责语义并按 #84 后 #45 集成，#45 复核 #84 的实际字段与接口版本。#44 仍以 #45 为硬前置；#45 经复审、必要验证和真实合并，结果同步且成果进入最新主干后启动 #44。已有在途执行先核对原 run 与固定契约，不因清单编辑自动中断或切换规格。
3. 后续界面任务留在[草稿](../../Roadmap/drafts/roadmap.md)，原型确认后再纳入。

## 待人工验收

| 范围 | 状态 / 安排 | 明细 |
| --- | --- | --- |
| 一期单安装版签收 | #83/#84 历史实现已交付；仍待当前支持路线实际能力及原生准备条件落实、固定新安装构建，再组织组合/24小时/同构建恢复人工签收；窗口待安排。当前 Orca 本机只有有限真实样本，本机最终 draft 保护未重新跑真机；远端升级 CLI/预先确认目录信任后有限样本闭环通过，首次信任阻塞后原任务自动继续未通过，见[最新范围](../../testing/acceptance/orca-terminal-remote-retest-2026-10-10.md)；旧 supervised 缺口[保留历史](../../testing/acceptance/worker-staged-flow.md#后续主干-orca-路线的当前能力缺口)，不以历史通过覆盖或改称人工豁免 | [一期活动](../../testing/acceptance/phase1.md) |
| #84 步骤辨识与反馈/合并 | WFS-M1 待验；本任务自动能力交付后，可与一期 P1A1-M 同一现场集中验收，分别登记结果；不阻塞无关二期研发 | [专属记录](../../testing/acceptance/worker-staged-flow.md) |
| #44原生输入与可达性 | #44已交付；UI2-M/UI4-M/UI5-M待人工，最迟2.1签收 | [记录](../../testing/acceptance/archive/superseded/phase2-winui-foundation.md) |

自动补验：[Codex race](../../testing/acceptance/local-codex-app-server.md) · [当前 Orca 单终端与远端待验](../../testing/acceptance/orca-terminal-transport.md)。旧[Orca原生编排](../../testing/acceptance/archive/superseded/worker-flow-orca-native.md)及[安全重建](../../testing/acceptance/worker-service-recovery.md)保留历史来源，适用性须按当前单终端重新核对，不能照旧路线直接执行。

## 执行记录

2026-10-10，#44 已经实现、独立复审、局部修复及新的独立终审，PR #273 于 UTC 05:54:56 真实合并；head `b3b52c24fba2a96a1746d3249756518dbf1f9d3b`、merge `f04fdf089a58723909279d44c497377427b868dd`。关闭草稿竞态由 `/root/fix44_round1` 修复于 source `f1846b9bd628f43603658d265db0364e1cb87750`，`/root/review44_round2` 终审无新确认阻塞缺陷；[实际审核记录](https://github.com/erranliu/SteerLine/pull/273#issuecomment-6094396874)。新编译定向测试 19/0/0、WinUI 构建 0 warning/error，独立复跑现有编译测试 19/0/0；不是新 source 真实窗口验证。历史 `c5c9eb47` 窗口、公共查询与清理证据保留原来源。UI2-M/UI4-M/UI5-M 按用户指示待人工、最迟 2.1 签收，登记 `passed_with_manual_pending`，未宣称全部验收完成。 [专属记录](../../testing/acceptance/archive/superseded/phase2-winui-foundation.md)。

2026-10-10，#44 按固定 Spec/Plan/共同约束 commit `550234ab0fb0aa74a3676ff09dde6e4bd4325295` 启动，初始集成 base `fbde4163a42639d5570dbe718b32cd42e95dc676`；随后将最新 `origin/main` `eb76c50d69637212b66ddb60990ff68415d67a11`（含 PR #275/#276）合入。最终实现候选 `c5c9eb470835c6ab28f7343b40b41f3648f086cf`（tree `b4a7a265c03a7aaecc9a749ba3d5eebab36f7df0`）。[PR #273](https://github.com/erranliu/SteerLine/pull/273) OPEN，未合并、不移除草稿任务。时间优先生成包 `C:\SteerlineDebug\dev-20261010-122449-c5c9eb47` source clean、冻结 simulate；真实 WinUI 使用配置 Git helper 连接公开 identity/tasks/detail API，并显示受控 worker 下 Task 1 `in_progress`。定向 WinUI 测试 15/0/0，HTTP API、task model、Codex runtime、process Go 包测试通过，构建 0 warning/0 error。UI2-M/UI4-M 和 UI5 worker close/reopen 同身份步骤按用户指示列为待人工，最迟 2.1 签收；当前证据不把 UI5 记为自动通过，不阻塞 PR 交付。final 包 console stop receipt `applied`，进程退出/锁释放，task/provider/Git facts 保留且未 reset；[验收与限制](../../testing/acceptance/archive/superseded/phase2-winui-foundation.md)。

2026-10-10，#84 post-merge 独立审计确认 PR #262 历史交付无阻塞缺陷；main 已通过 PR #268 在 execution-map/验收中登记 #84 实现完成，WFS-M1 和后续 Orca supervised 规范 final 能力缺口仍分别待人工/技术补齐。#45 将最新 main `40446a671badc901bcf868872a383b421d646803` 以 merge `5bee8ded64a79982e4c39688c3e2cff595800be1` 集入原 PR 分支；精确候选 `21fb131f6b89cc6e04c3b85a4a9e3de232c2b613`。stale task version、unsupported profile/receipt、18 HTTP/API、8 服务集成、staged query/feedback、vet 及 L1（12组/479通过/0失败/0跳过）通过。API2 原始 SHM failure 仍 `验收失败`；限定风险接受未生效，等待 #45 新独立终审。PR #249 保持 OPEN，#44 不启动。

2026-10-09 #84 集成主干原生 Orca 改动（PR #259，主干 ade7042b），沿原固定 c4bcbec Spec/Plan 实现通用步骤，未恢复退役 launcher。用户明确调整 Orca usage 验收为公开接口不可用如实登记，真实三轮/原身份/PR/清理仍必验；[专属记录](../../testing/acceptance/worker-staged-flow.md)同步原失败与新验证范围。历史预留 #86 保留身份但不进入 DAG，主干直接实施结果见[原生 Orca 调整](../design/orca-native-worker.md)，不作为 #84 等待边。


2026-10-09 最新授权：“rebase，#84任务可以并行执行了, #84用 GPT6.1 Sol High”。#84 开发聊天使用 gpt-6.1-sol/high（产品 E2E 仍按固定 Plan 的原模型/原生权限），与 #45 独立并行；共享主干接线先 #84 后 #45，#44 硬前置保持。#84 固定 Spec/Plan commit `c4bcbec06289c9f5b086c8f1f98859a0f858d7e5`，代码起点同 SHA，工作树 b123、分支 codex/task-84-worker-staged-flow；实现/验证进行中，尚未独立复审或合并。#45 原执行者仍处理 PR #249，API2 失败未闭合，不由本任务接管。此前串行纳入记录是历史安排，不覆盖本段最新授权。


2026-10-09，用户确认将 #84 纳入 execution-map，沿用已预留身份及独立 Spec/Plan/专属验收。状态为待执行、尚未启动，产品自动与人工项仍未验；#83 已合入，无未完成硬前置。按一期收尾安排，剩余串行顺序为 #84→#45→#44；#45→#44 的硬依赖保持，共享接线排期不新增业务依赖。文档版本在实际启动前提交并固定，不创建外部 SteerLine Task/DAG。

2026-10-08，用户确认采用 Codex 直接开发流程；#83/#45待执行，#44等待#45，当前未启动、未验。外部中心配置问题不作为本流程前置；具体产品测试的环境要求仍按 Spec/Plan 核对。历史查询与本次流程调整见[记录](../../testing/acceptance/roadmap-admission.md)。

2026-10-08，独立复审 round 1 在 [PR #247](https://github.com/erranliu/SteerLine/pull/247#issuecomment-6058531009) 发现 P1/P2；P1 局部修复提交 `74ea6413214d980bd0bb1052c8b3ed5d8811669d`，原执行者继续补 P2，最终复验待做。详见[验收记录](../../testing/acceptance/local-runtime-isolation.md)。

2026-10-08，候选 `066a5985677cccb4fdb4650bf669ddf15050de90`（tree `98856831907b85a37ba8b5f3364726b0659de358`，Windows/amd64 clean tree）的原 runtime runner、P1 子集、L1、`delivery.basic` 与 Linux/amd64 编译结果保留各自已测范围；round 2 后确认 LI3-A 因缺省 state/UI junction 实际写入安装侧而失败，LI2-A 因未执行 generated batch/service 入口而待验。见[任务验收记录](../../testing/acceptance/local-runtime-isolation.md#2026-10-08-独立复审-round-2-缺省写路径失败及-batch-未验)。PR #247 未合并；#83 保持执行中。

2026-10-08，修复候选 `ad22d476f87e019a9ad5d0118758b253dd091094`（tree `e2470c18a0d29979006aa5e12a440f2035bf5fa0`，Windows/amd64 clean source）通过 LI1-A–LI4-A：state/UI junction 的脚本与 native 入口在副作用前拒绝；`Operation service`、直接 generated `service-window.bat` 和 generated `console.bat` 均真实运行并记录进程/路径/实例/停服回执；P1 10 项、L1、delivery.basic、owner/boundary 定向、ConPTY smoke 和 Linux/amd64 编译通过。证据见[最终验收记录](../../testing/acceptance/local-runtime-isolation.md)。PR #247 未合并；#83 保持执行中并等待新独立复审。

2026-10-08，round 3 独立复审候选 `f5c729f985f56f1a59c7506a6bc07dfc6e9ab402` 确认 direct build.ps1 漏 canonical containment（LI2-A 失败）及 stale instance/config 请求计数错误（LI3-A 该子项未有效验证）；撤销此前全部通过汇总，历史原始证据保留。独立局部 fixer 仅修 runtime 请求观察及真实正常发布的计数增长断言；一次受影响 controlled 场景通过并清理完成（HEAD f5c729f 加局部测试/结果文档修改、source_dirty=true，测试 blob 072258ca）；两条拒绝 0→0，真实正常发布 0→2。P1 留给原执行者，最终候选及新独立复审仍待完成。PR #247 未合并、不释放后继；[明细](../../testing/acceptance/local-runtime-isolation.md#2026-10-08-独立复审-round-3-请求观察局部修复)。

2026-10-09，原执行者候选 `f838dcf587efb2cb22992e6354640f530e0df8b7`（tree `deb3ae0f1dbc3b2a9402e0ecc0681a2a5beb4682`，Windows/amd64 clean tree）修复 LI2-A direct `build.ps1` canonical containment 和 LI3-A 正式 request 计数断言；同一受控 L3 runtime 通过，1 test/0 skip/清理完成，P1 10 项、L1（3 workers/12 groups/426 tests/0 skip）、`delivery.basic` 与 Linux/amd64 编译通过。LI3-A stale instance/config 两条拒绝均 0→0，正常产品领取正式 request 0→2。LI2-A 源码 bin junction 与输出 hardlink 均拒绝，普通构建/重建成功，安装 exe/DB/task facts/bin/service instance 保持。前序失败运行的 Windows scratch 路径、Git line-ending 环境和 CLI dirty 字段断言原因、产物定位见[验收记录](../../testing/acceptance/local-runtime-isolation.md)。PR #247 仍 OPEN、未合并；#83 保持执行中并等待新的独立复审，不释放后继。

流程规则见[RoadmapGuide](../../Roadmap/RoadmapGuide.md)；[二期节点验收](../../testing/acceptance/phase2.md)目前仅规划。

2026-10-09，round 4 受审 HEAD `054decf` 的 SQLite SHM 共享硬链接真实负例失败，LI3-A 整体已验汇总撤销；临时 installed 哨兵变为 32768 字节，产品 CLI/wrapper 最终 invalid_batch 不构成副作用前拒绝。当时局部修复与精确候选复验待完成；[明细](../../testing/acceptance/local-runtime-isolation.md#2026-10-09-round-4-独立复审sqlite-共享硬链接失败)。PR #247 未合并、不释放后继。

2026-10-09，round 4 局部修复生产 commit `9aeed603d46bf7d566b2daf183855dc3c8316fc4` 拒绝 SQLite DB/WAL/SHM/journal 共享硬链接，Windows 真实 link count 在 app/host/reader/writer SQLite 打开前核验。最终干净候选 `92eaf9b8163f71be0554265ddba85ee0708855ee`（仅场景时点调整，tree `28f8395f90b71d3f8a7d0dc139c43d17dbbd2345`）完整 L3 runtime 1 test/0 skip/262.63 秒通过并清理；六条产品 CLI/debug wrapper 硬链接负例在副作用前明确拒绝，哨兵与全部 SQLite 快照逐次不变，普通单链接仍可用。`9aeed603` 定向 9 个顶层测试、L1 12 groups/426 tests/0 skip、Linux/amd64 编译通过；之后生产 Go 无变化，回归精确映射复用；旧 `f838dcf` delivery 只保留原范围。首轮 runtime 的运行中 installed DB 字节比较失败及清理证据保留，新的独立目录复验通过。LI3-A 已按新证据修正为已验收；[明细](../../testing/acceptance/local-runtime-isolation.md#2026-10-09-round-4-局部修复与精确候选复验)。PR #247 仍未合并，#83 执行中并等待新独立复审，不释放后继。

2026-10-09，第五轮独立 reviewer `/root/review83_round5` 未发现新产品问题或固定需求遗漏，协调核对候选、实际生产路径和精确验证映射并在 [PR 评论](https://github.com/erranliu/SteerLine/pull/247#issuecomment-6065781425)登记。GitHub 确认 [PR #247](https://github.com/erranliu/SteerLine/pull/247) 在北京时间 2026-10-09 01:50:18 合并，审核 head `2ce81db45d7bff32a1ffb6134210995529f01b84`，merge commit `f7c6f1d05c78172fe8133f7c1763718df0165fe1`，成果已进入默认主干。#83 实现完成；自动验收 LI1-A–LI4-A 已通过，无任务人工项。一期组合/24小时/同构建恢复签收仍待安排，未宣称完成。可选文件符号链接负例未运行，复审临时探针清理被自动审批拒绝而残留三个合成文件的事实分别见[验收明细](../../testing/acceptance/local-runtime-isolation.md#2026-10-09-交付结算与独立复审)。本结算记录进入主干后按原序启动#45；不提前启动等待#45的#44，也不扩大到草稿任务。

2026-10-09，用户明确授权本聊天直接执行 #87，不经 roadmap-runner。沿用原预留身份及本轮 worker 停服行为/统一恢复 Spec/Plan；原生事实由 worker 实现核实，应用只消费规范事实。无未交付硬前置，当前主干含 #84 规划文档及另项等待合并反馈修复（PR #253），#84 分步执行本体尚未实现；本任务不依赖该未交付能力，也不改其历史验收汇总。启动时提交并固定文档版本，分支 codex/worker-stop-recovery，代码起点 d8d66d6c；安装数据与服务不纳入修改范围。

2026-10-09，#87 最终自动候选 `399c4485` 完成双真实路径及必要定向验证；独立 reviewer `/root/review87` 对代码与最终证据汇总 `187dd098` 复审 clean。GitHub 确认 PR #260 于北京时间 16:39:52（UTC 08:39:52）合并，PR head `187dd098b9c2c2b11912770fdc70fdfce6f06255`，merge `d951ca8729d5854b8429b7790fef8ff75feaa750`；已 fetch 并核验该 head 为最新默认主干祖先。#87 实现完成，WSR-A1–A8 自动通过，无任务人工项；精确候选、历史失败及未重跑范围见[验收记录](../../testing/acceptance/worker-stop-recovery.md)。已移除草稿任务条目，保留能力摘要及身份历史。安装 101、安装替换及一期长期签收保持独立未验范围，不修改 #84→#45→#44 排期或外部 Task/DAG。
