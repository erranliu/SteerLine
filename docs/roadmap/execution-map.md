# 正式执行 Roadmap

开发执行方式：roadmap-runner 直接使用 Codex；本表是用户确认的开发任务与依赖权威。

## 当前任务

| 任务 ID | 任务名称 | 前置 | 状态 | Spec / Plan | 验收 |
| --- | --- | --- | --- | --- | --- |
| 83 | 本机安装与调试路径隔离 | 无 | 实现完成 | [Spec](../spec/local-runtime-isolation.spec.md) · [Plan](../spec/local-runtime-isolation.plan.md) | [PR #247](https://github.com/erranliu/SteerLine/pull/247) 已真实合并，merge `f7c6f1d0`；第五轮独立复审无新问题；LI1-A–LI4-A 自动验收通过，无本任务人工项；[记录及验证范围](../acceptance/local-runtime-isolation.md) |
| 84 | Worker 分步执行与结构化状态推进 | 无；复用已合入 #83 的隔离基线 | 待执行 | [Spec](../spec/worker-staged-flow.spec.md) · [Plan](../spec/worker-staged-flow.plan.md) | 产品自动 WFS-A1–A4 未验；WFS-M1 可后置至一期组合验收；[专属计划与记录](../acceptance/worker-staged-flow.md) |
| 45 | 本机客户端协议与历史查询 | 无 | 待复审/合并 | [Spec](../spec/phase2-local-api.spec.md) · [Plan](../spec/phase2-local-api.plan.md) | [PR #249](https://github.com/erranliu/SteerLine/pull/249) OPEN；最新整合候选 `8cd41990`（tree `9372f76a`），API 源码候选 `99c98e2b`（tree `d8b7fad5`），新测试/Plan 候选 `f82c7387`（tree `77d29630`）。API1/API3/API4/API5 自动项通过；API2 新增非默认 profile 实际 turn/Decision、冻结 digest 映射及受控 provider PR/Delivery HTTP/Query parity；host state/Orca target/preflight 组合测试通过；L1 12 组/458 项/0 skip。旧 `-shm` AccessDenied 仍未证实具体 root cause，API2 保持验收失败并待独立 reviewer 裁定；[记录与证据](../acceptance/phase2-local-api.md) |
| 44 | WinUI 基座与原生输入 | #45 | 等待 #45 | [Spec](../spec/phase2-winui-foundation.spec.md) · [Plan](../spec/phase2-winui-foundation.plan.md) | 未验；[记录](../acceptance/phase2-winui-foundation.md)，人工项最迟2.1签收 |

## 推进顺序

1. #83 已合入；剩余按 #84 → #45 → #44 串行推进。启动时使用已提交且执行工作区可读取的 Spec/Plan commit 和路径，并核对最新默认主干；本次纳入清单不等于已经启动。
2. #84 与 #45 无新增业务硬依赖，共同状态/存储/宿主/客户端接线按上述顺序集成，#45 复核 #84 的实际字段与接口版本。#44 仍以 #45 为硬前置；#45 经复审、必要验证和真实合并，结果同步且成果进入最新主干后启动 #44。已有在途执行先核对原 run 与固定契约，不因清单编辑自动中断或切换规格。
3. 后续界面任务留在[草稿](drafts/roadmap.md)，原型确认后再纳入。

## 待人工验收

| 范围 | 状态 / 安排 | 明细 |
| --- | --- | --- |
| 一期单安装版签收 | 隔离能力 #83 已合入；待 #84 交付及最小自动验证、固定新安装构建并满足原生准备条件后，组织组合/24小时/同构建恢复人工签收；窗口待安排 | [一期活动](../acceptance/phase1.md) |
| #84 步骤辨识与反馈/合并 | WFS-M1 待验；本任务自动能力交付后，可与一期 P1A1-M 同一现场集中验收，分别登记结果；不阻塞无关二期研发 | [专属记录](../acceptance/worker-staged-flow.md) |
| #44原生输入与可达性 | #44已排入；交付后验UI2-M/UI4-M，最迟2.1签收 | [记录](../acceptance/phase2-winui-foundation.md) |

自动补验：[Codex race](../acceptance/local-codex-app-server.md) · [Orca原生编排](../acceptance/worker-flow-orca-native.md) · [Orca安全重建](../acceptance/worker-service-recovery.md)。

## 执行记录

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

2026-10-09，按 #83 合入后的最新主干 `f628161b5a60cefeb75922c15a63b1df9ef9c21b` 执行 #45；Spec/Plan 路径与 commit 在验收记录固定。实现候选 `c18acecc195d294c4b2e7250ad2075224e73bdfa`（tree `c1f9ca9c42bed5ada307b6f6b83b7e55836a98a7`），已提交 [PR #249](https://github.com/erranliu/SteerLine/pull/249)，base `main`，当前 OPEN、待独立复审，未合并。旧候选实际所跑 API 定向及六项真实 Windows 服务集成通过（round 1 覆盖遗漏见后续纠正）；L1 12 组/427 tests/0 skip；Go vet 与文档静态通过。首轮集成出现一次 SQLite SHM `Access is denied`，单项重跑及第二轮完整集成通过；此前宽范围 app/execution 测试曾 10 分钟超时，原失败和边界均登记于[专属验收](../acceptance/phase2-local-api.md)。本状态仅为待复审/合并，不记实现完成；#44 继续等待 #45，不释放后继。

2026-10-09，#45 round 1 独立复审确认 API1 临时私有 ACL/preflight 竞态和 API3 同请求异内容权威误匹配；API2 profiles 查询及真实 API1 mutation/API3 故障驱动覆盖遗漏待补，原自动全部通过声明撤回。旧证据按原实际覆盖保留，SHM AccessDenied 根因未闭合。PR #249 未合并，局部修复与原执行者续验后须新独立复审；#44 仍等待。[明细](../acceptance/phase2-local-api.md#round-1-独立复审确认)。


2026-10-09，#45 round 1 局部修复生产候选 `ac3c6411398da955b535cbc87df9fcfff4b8c9a8`（tree `2fff55b07d57707bb46b8688841ad3ec2b190e82`）已在精确干净提交通过 14 HTTP、3 app、2 query 定向（0 skip）及 vet。matcher 绑定首次注册时的实际语义，正式同 ID 消费者竞争同内容可对账、异内容明确 identity_conflict；Windows 两个 writer 在 temp 仍打开时的真实 preflight 正向通过，继承/Everyone ACL 负向仍拒绝。后续 docs-only 提交与该受验生产/测试源码一致。原执行者需继续 profiles、真实 API1/API3 覆盖和 SHM 根因未闭合项，之后新独立复审；PR #249 未合并，#45 执行中，#44 仍等待。[局部证据与剩余项](../acceptance/phase2-local-api.md#round-1-局部修复与精确复验)。

2026-10-09，#45 round 2 当前候选 `99c98e2bd02b3c0f75fbdea4919ce54e59927745`（tree `d8b7fad5eff3db07cdb7b5f544c0b68c2509a36c`）补齐 profile/presentation 消费、真实 API1 mutation/credential rotation 和 API3 control inbox 取消/丢回执/迟到竞争驱动；独立 SQLite sidecar bounded retry 仅对短暂 `ERROR_ACCESS_DENIED`，持续 ACL refusal 与 hardlink 拒绝保留。候选 L1 12组/428 tests/0 skip、真实 API1–API5 场景组7项/0 skip、QueryParity 独立 count=3、pathguard真实 sidecar/ACL regression 与 vet 通过。旧 QueryParity `-shm Access is denied` 日志真实保留，但精确 OS syscall 阶段与根因未能在现场复现；API2 仍标验收失败，不宣称全部自动项通过，等待新独立 reviewer 判定本轮证据能否闭合。PR #249 未合并；本表仅待复审/合并，不记实现完成；#44 继续等待 #45，不释放后继。详见[round 2 证据与 SHM 未决项](../acceptance/phase2-local-api.md#round-2-真实消费与故障窗口)。

2026-10-09，main 前进到 `04c62ee5ddb10637a68745baa4fc4f811123ffa9` 后，以 merge commit `08341fbc90e975ed512e47740860f2d2e25edb1d`（tree `ba3e2837dd616ebc8789bbc2a56771e281a2216d`）整合到 #45 原分支。差异仅为 `internal/console/model_control.go` 和其 start-request tests；API/host/SQLite/HTTP 源码 unchanged，round 2 API1/API5 实证映射复用 `99c98e2b`。console 5 个 start/profile 相关 tests 和 vet 通过；L1 在精确 merge commit 12 groups/441 tests/0 skip。API2 的旧真实 SHM failure 仍未闭合，按验收文档保持失败、供新独立 reviewer 裁定；PR #249 未合并，#44 仍等待 #45，不释放后继。

2026-10-09，main 再前进到 `7ffc6a39197366b1177e104eb75cbc453ccee78a`（PR #250：merge-waiting 与 optional worker feedback 状态区分），以 merge commit `c70414718ec3593b00d1c911ea1894b76614e1a2`（tree `55d50d0b2f0bdea44a80901cd772903560742aac`）整合。相对 `08341fbc` 只修改 8 个 console 状态/反馈显示文件，API/host/SQLite/HTTP 生产源码无变化。`TestWaitingPRMerge*` 与 restored start request 定向通过，console vet 通过；精确 merge commit 的 L1 为 12 groups/458 tests/0 skip。API1–API5 的既有证据映射仍使用 `99c98e2b`；API2 `-shm Access is denied` 历史根因仍未证实，验收状态保持失败，等待新独立 reviewer 裁定。PR #249 未合并，#44 仍等待 #45，不释放后继。

2026-10-09，main 再前进到 `692a784e05479b11e5818fb5a209971949e3064a`（PR #251: Orca native target 与 worktree-trash metadata），以 merge commit `8cd4199034502642a35576b88226d3ada5ac2583`（tree `9372f76a4d57c3f0be3517d8cf5f722c86ebb62d`）整合；#45 增加测试/Plan 候选 `f82c73876e9d1dfae8be380348bd7f5d7db27fc9`（tree `77d296303f5caba8e327e7c21dada626bd0d3c24`），未改 Spec。Host state/Orca target/preflight、配置服务 discovery 受影响定向均通过。API QueryParity 现从 `presentation` 选 `fixture-alt`，HTTP control request 保留该 profile digest，真实 backend turn/Decision 的 frozen WorkflowRun profile payload 匹配 alt；查询 digest 与 claim-time frozen digest 按原语义分别核验（本次值不同）。受控 provider 正式 merge 生成的持久 Delivery 经 HTTP ShowTask/Snapshot 与 native Query 对比通过。新增两次测试 fixture 失败日志保留并已修复；最终 QueryParity、Delivery 子场景与精确候选 L1（12 groups/458 tests/0 skip）通过。API2 的旧 SHM 失败具体系统阶段仍未知，不作 root cause 证实/环境豁免，保持验收失败；PR #249 不合并，#44 继续等待。[round 3 证据](../acceptance/phase2-local-api.md#round-3-latest-main-非默认-profile-与持久-delivery-查询)。
