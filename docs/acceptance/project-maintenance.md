# 项目基线与材料统一维护验收

专属任务：[Spec](../spec/project-maintenance.spec.md) · [Plan](../plans/project-maintenance.plan.md) · [统一设计](../product-design/project-maintenance.md)。2026-10-21 已按用户授权实现并验证；当前记录与 2026-10-20 的文档审查历史分别保留，无 roadmap Task ID。产品结果见本文实施验证节，人工体验 PM-M01 后置。

## 自动验收

各项具体环境、命令、时点及入口补齐安排见 Plan 测试表。实施时记录源码 commit/dirty、Runner 版本/SHA、规则版本、包/证据目录、实际选择、原请求、结果及清理；未执行/零匹配/环境不足/缺证据均不得标通过。

### PM-A01

远端周期/手动/首次观察、合并在途读取、删除/非快进及失败时间诊断正确；只提示用户正常 Git 更新，不实现产品 sync，检测不生成或改本地分支/工作区。状态：自动验证通过（2026-10-21，证据见实施验证节）。证据：`remote` 定向报告、Git 调用与前后 refs，controlled 页面状态。

### PM-A02

初始化/基线/局部只改范围/来源，复用窄规则；缺基准/增删改、三个完整输入新鲜度、CRLF草稿提交后的继承及范围正确，无生成自循环或提前建库/完整纳管门槛。状态：自动验证通过（2026-10-21，证据见实施验证节）。证据：`rules` 报告、规则命中、临时仓库/DB 调用事实。

### PM-A03

人工采用及 Task 内容确认有固定实际输入、差异和 CAS；过期/缺引用拒绝，不可借确认改变 repository/remote/目标分支。源码适用性只提示，必要材料待办才阻断相关首次启动；外部材料可人工采用，无需先调用模型。状态：自动验证通过（2026-10-21，证据见实施验证节）。证据：`rules`/`freeze` 报告、确认前后版本及摘要。

### PM-A04

固定完整输入/profile/原信封，一次只生成一个Plan/一个输出；新旧入口均先待审阅，不提前写入或附带测试、Git发布、登记/派发。草稿可生成/应用但未合入不就绪。状态：自动验证通过（2026-10-21，证据见实施验证节）。证据：`generation`/`contracts`及live原Run/信封、一次调用计数、应用前文件摘要。

### PM-A05

单文件应用的输出/HEAD/分支/输入/目标条件及路径保护正确；取消先/应用先、取消后旧号不得新写、丢回执/重启只核验、旧输出重新审阅后显式复用可控。无多文件事务；已应用/待提交/远端就绪分开。状态：自动验证通过（2026-10-21，证据见实施验证节）。证据：`generation`/controlled/live报告、原单文件应用事实、远端blob与材料依据。

### PM-A06

未知原请求只对账；取消/迟到停止义务保留，明确失败的新尝试核验原终态/资源资格；重启零自动生成/应用，所有真实生成复用原围栏，不虚构预纳管生成。状态：自动验证通过（2026-10-21，证据见实施验证节）。证据：`generation`、回归、live原请求、重启前后计数及公开停止/结算记录。

### PM-A07

各首次启动入口冻结fresh fetch同commit全refs/base/测试；原项目库内相关确认/Task/图/目标竞态零派发，无关Plan/轮询时间不冲突。同内容新commit无须人工重确认；有效空安排允许。首次预留后拒绝/未知/退出/后续候选共用原版本，历史改写排除原base时拒绝新候选，不换base，Delivery保持。状态：自动验证通过（2026-10-21，证据见实施验证节）。证据：`freeze`/SQLite/CLI/API报告、事务前后/原提交及controlled执行页。

### PM-A08

新独立包真实Go/Web/Edge完成三入口范围调用（初始化为服务适配）、基线/Git更新提示、单项生成/审阅/应用、隔离remote实际提交后核验、首次冻结/PlanTests，跨项目不串授权。一次真实Runner/Codex走共用路径。状态：自动验证通过（2026-10-21，受控 v6 与真实 live v3；失败轮次单独保留）。证据：`flow`、trace/截图/API、`live`及owned资源清理；controlled不冒充真实模型。

## 人工验收

PM-M01：自动项通过后，用户在新独立包核对提示、影响清单、diff 和启动待办是否易懂；参与窗口届时安排，可后置，仅阻塞体验签收，不阻塞无关任务。状态：待人工。

## 现场与清理约束

产品测试先由唯一 `scripts/new-debug-release.ps1` 建 `C:\SteerlineDebug` 新包，测试项目只用 SteerlineTest。受控场景用本地 bare remote 和显式 `--simulate`，真实 generate 用固定发布版 Runner/本机 Codex；不使用安装/保留运行环境或复制其数据库/认证/配置。首版无需真实 PR；既有 live 驱动如产生其专属远端 base/head，按原归属/精确 SHA/lease 清理，不影响其他分支。

所有原生成、未绑定/迟到 Run、应用/项目/共享服务退出分别取证，停止受理不代替资源结算；浏览器页面/生成成功不代替 Task/Delivery 事实。失败与补测分别记录，Plan 不回填实时结果。

## 首轮草稿文档检查（审查前）

2026-10-20 仅新增/同步 16 份 Markdown 文档，产品 PM-A01–PM-A08 均未执行。`python scripts/check_doc_links.py` 检查 503 份文档、3172 条本地链接，0 错误；`git diff --check` 通过，并额外检查新未跟踪文档空白。静态映射核对 PM-A01–PM-A08 全部进入 Plan 测试计划及本页，人工 PM-M01 单列；17 处本次相关锚点可解析。只改 docs 下 Markdown，未启动服务、生成模型、同步目标仓库、改安装状态或登记任务；文档检查不计产品自动项通过。

## 2026-10-20 文档审查与修订

用户直接授权审查并修正文档，未启动实现。确认并修复以下问题；这些是设计修正，不计 PM 产品验收通过。

| 问题 | 最简修正与对应验收 |
| --- | --- |
| 初始化复用被扩大成提前交付 PS 全套核验、预纳管候选库，形成职责和前置混淆 | 三入口共用窄快照/规则；完整 PS/登记仍归项目添加，初始化仅服务适配测试。PM-A02/A08 |
| Git同步、通用维护/Task修订CLI、批量生成及多文件事务超出首版需要 | 只提示用户pull；Web+原tests CLI，单Plan/一次调用/单文件应用，无批量事务。PM-A01/A04/A05 |
| 模型读完整清单，但过期检查只看ID集合/所选定义，会错误沿用结果 | 统一按完整实际输入/规则判断，重核可人工采用，不强制重生成。PM-A02/A04 |
| 草稿CRLF与提交LF、HEAD改变时的旧结果处置未闭合 | 依赖摘要仅归一化换行，原信封/Task SHA/写入条件仍真实字节；重新审阅可用新应用请求复用适用输出。PM-A02/A05 |
| 任意源码更新都可能触发全局人工确认，阻碍后继自动启动 | 源码适用性只提示；实际材料缺失/无效/变化未复核才阻断相关Task，无关Plan不阻断。PM-A03/A07 |
| 应用级文件确认与项目库首次冻结无法真正同事务，轮询/无关Plan也可能误冲突 | 已配置项目依据与首次预留在同一项目库重核，只比较消费的确认；Task目标不可借确认更换。PM-A03/A07 |
| 取消/应用竞态、重启未写状态和单独输出摘要不足以证明审阅现场 | 取消与应用意图有唯一顺序，取消后旧号不新写；review_digest覆盖完整现场，查询只核实不补写。PM-A05/A06 |
| 后续候选是否重新解析最新材料、远端改写后的祖先条件未明确 | 先取原Task快照，再核验原资格/祖先；原base被排除则拒绝新候选，拒绝/unknown/退出均不解冻。PM-A07 |

已同步相关项目添加/结构、测试管理、数据模型和 W/E/V 设计。自动项 PM-A01–PM-A08 均保留待实现/待验，测试计划覆盖修订的正反例，人工项后置。本次 `python scripts/check_doc_links.py` 为 503 份文档、3165 条本地链接、0 错误；`git diff --check` 及新未跟踪文档空白检查通过，8/8 自动项映射完整、15 处相关锚点可解析。累计改动仍仅为 docs 下 16 份 Markdown；没有运行产品测试、创建执行环境或修改安装/远端状态。


## 2026-10-21 实施验证

实现源码基线 `9e02854ef66e1b1e4b1fbdff35a9eeca294b26a5`，分支 `codex/project-maintenance`，工作区 dirty（未提交）。规则版本 `project-maintenance/1`，SQLite schema 26；不提供旧开发数据迁移。本批未部署到安装目录，未运行全量测试、全量 L2/L3 或真实发布 PR。

### 定向验证与对应验收

以下均为本次实际报告，不能累加为互不重复的测试数量；`summary.json` 保存实际命令、选择、源码/机器和逐项结果。证据根为 `C:\SteerlineDebug`。

| 验收项 | 实际入口与结果 | 证据位置（相对证据根） |
| --- | --- | --- |
| PM-A01、A02、A03、A07 | 远端 fresh fetch/改写/失败与在途合并、三入口纯规则、完整输入/CRLF、确认与冻结、相关 approval CAS、原候选/Delivery；定向收尾 9 包 60 条记录通过（含子测试），无跳过 | `pm-evidence-20261021-final-check/summary.json`、`go-test.jsonl`；最后简化观察接口后 `go test ./internal/app -run '^TestPM' -count=1` 通过 |
| PM-A04、A05、A06 | 原生成/原请求/unknown/迟到停止回归；待审阅零自动保存，HEAD/远端目标/原字节与输出保护，取消及未写意图/丢回执/持久化失败只核验，新的尝试不能替代未结算的原请求 | 同上 `final-check`，及 `pm-evidence-20261021-regression-final/summary.json`（6 包 251 条记录）、`pm-evidence-20261021-l1-final/summary.json`（6 包 34 条记录） |
| PM-A03–A08 消费者与边界 | API/CLI/应用入口及依赖边界定向通过；Web 4 文件 23 项通过，覆盖面板/PlanTests/切项目；未用 mock 冒充 Go 闭环 | `pm-evidence-20261021-consumers-final/summary.json`（6 包 121 条记录）；`pm-evidence-20261021-ui-final.log` |
| PM-A01–A05、A07、A08 受控闭环 | 正式 L3 `runtime.project-maintenance` 通过，465.25 秒。真实 Go/Web/Edge、双项目及独立 bare remote；生成待审阅/显式应用、更新远端/Git提示/采用、Task确认/首次固定/后续候选原版本/PlanTests与跨项目隔离 | `pm-evidence-20261021-flow-v6/L3/summary.json`；包 `dev-20261021-102638-9e02854e-dirty/evidence/project-maintenance/` 下 `proof.json`、`reset.json`、浏览器 facts/trace/截图 |

受控场景由唯一生成器建立新包并显式 `--simulate`，不计真实模型通过。`proof.json` 的 `passed`、`stopped`、`resources_settled` 均 true；停止/资源分别检查。reset 之后各项目 Task=2、contract=2、execution snapshot=0，保留定义及真实 Git 事实。确认过期信封拒绝且零写入；首次固定 commit 同时出现在测试快照/API/执行页，材料随后变化不会替换后续候选的原版本。

### 真实 Runner 与模型现场

固定发布版 Runner 为 `0.2.0-dev.3-r2`，源码 `8261310d5f2d1edb6fa0c81ba05fa4119b561386`，资源摘要 `a48bfb1aa7250009b21b4170532d877be01c23375ac386cb39e269794263ef91`，EXE SHA256 `4dbf25c74ecc5d360fbfacc88fe599744cdc80a10648962ddc86f73c7d4b467b`；源文件 `C:\Project\SteerRunner\bin\steerrunner.exe`，不读取安装环境。各现场原 pin/信封/结果位于独立包证据，源码为上述 dirty 工作区；不宣称为干净提交的受管通过。

首轮 live：`pm-evidence-20261021-live/proof.json`，包 `dev-20261021-103522-9e02854e-dirty`。真实 generate Run `41ead3dc3fd994146dd11ff47ff31671`，来源 `codex.agentMessage.final_answer`；应用前文件不变，显式应用后提交 `dd7254ff1aca9757f39955ae63e1e034b89b9566` 并采用。原候选 Run `7a920e978744a45d9adabb7c9128c2c0` 的外部程序实际产生通过/失败，API及浏览器均显示正确；停止与 owned refs 清理完成。**该轮有人工驱动介入**：旧驱动仅等 blocked，原生公开事实为 `reconciling/result_committed/implement.report` 且最终来源已验证；对原 Run 用公开 stop 请求 `fixture-collection-stop-6e0ad2f9de764ffdad35db87a2b11980` 后继续收集，保存原请求/回执。该事实不伪称为全自动闭环，不代表候选业务成功或发布完成。

驱动已按上述已提交且不可 reply/resume 的报告节点识别外部测试收集时点，继续要求验证来源；实际 pass/fail 仍来自程序运行，非模型文字。修正后独立现场自动复验记录见下文。

### 失败与补验

受控 v1–v5 的夹具/驱动失败分别保留，不回写为通过：能力信封漏 `error:null`、双项目默认选取、Playwright 相对 API 地址、重复重核引起超时，以及候选列表/初始 control 状态断言错误。修复测试适配并减少同一请求内重复 blob 读取后，v6 正式入口通过。生成包的 `npm ci` 曾与组件测试争用依赖；互斥后 23 项组件测试通过。首轮 live 的等待条件问题如上单独登记，另建新包复验，不在启动后切换执行方案。

PM-M01 仍待用户体验签收，不把自动浏览器截图当作人工验收。


补验二：`pm-evidence-20261021-live-final/proof.json`，包 `dev-20261021-104604-9e02854e-dirty`。原生成 ID `72f29ee3d2f644e69b90c374d12b2241`、Run `2430dbdf9819deaa6c285d69fc38b754`，模型返回 `{"tests":[]}`，与夹具要求的 `fixture.marker` 不符；产品停在 `needs_review` 并保留原文件，驱动明确失败，没有自动采用空结果、绕过确认或派发候选。`stopped`、`cleanup_complete` 均 true；不计 live 通过。原日志位于包内 `evidence/test-management/`。这是模型内容未满足验收，不改写为协议或业务通过。

最终静态检查：`npm --prefix web run build`（TypeScript + Vite）通过；新增面板定向 Vitest 3/3 通过。`python scripts/check_doc_links.py` 检查 506 份 Markdown、3176 条本地链接，0 错误；`git diff --check` 通过，26 个新文件额外空白检查 0 错误。文档/构建结果不替代产品验收。


### 最终独立 live v3（通过）

实际命令：`./scripts/test-test-management.ps1 -SteerRunnerExe C:\Project\SteerRunner\bin\steerrunner.exe -Out C:\SteerlineDebug\pm-evidence-20261021-live-v3`，退出码 0。唯一生成器创建全新 live 包 `C:\SteerlineDebug\dev-20261021-104919-9e02854e-dirty`，本轮没有人工推进 Run 或替换执行方案，也没有对原生成请求自动重试。

- 原生成 `49e01a15f8e1418c92d1eb41fa9d792d`，Run `93baadda98006c4123ca1d38fdf20e1a`，原生来源 `codex.agentMessage.final_answer` 且 verified。`generation-before-apply.json` 为 generated/applied=false，显式应用后的 `generation.json` 为 applied/applied=true，生成前后文件一致性检查通过。
- 采用实际材料后首次启动固定 commit `21520d1c922a49ac2f65be395649ca7cdb71299f`；候选 Run `b4dd64232c62e6f989d6c3e21643967d` 保持原材料/测试。实际程序的通过报告 `aca5c2a2e5b34a5fbf303398a118e91a`、失败报告 `6752293d155c4ac5b7c4beffe287d44a`，均为干净原 commit，退出码、summary、托管报告一致，API/浏览器两种状态均通过。本轮不宣称开发业务成功或完成发布 PR。
- 原 Run 的公开停止与结算检查完成，`proof.json` 的 `browser_passed`、`browser_failed`、`stopped`、`cleanup_complete` 均 true。`owned-refs-cleanup.json` 核实本包 base `codex/debug/dev-20261021-104919-9e02854e-dirty/base` 与唯一原 Run head `codex/runner-b4dd64232c62e6f989d6c3e21643967d`，无 PR 历史，按精确 commit/lease 删除后再次查询为空。没有清理其他项目/分支。

完整证据：`C:\SteerlineDebug\pm-evidence-20261021-live-v3/proof.json` 及包内 `evidence/test-management/` 的原请求/生成/应用/候选、程序/API、两套 Edge 截图/trace、停止/verify-stop 与清理日志。发布版 Runner pin 与前述相同。前两轮的介入/空输出事实仍保留，不能被本轮结果覆盖。

收尾修正纯提示文案：HEAD 与远端 commit 不同统一显示“本地与远端基线不同”，避免把本地领先或分叉一概描述为远端领先；仅文字和对应断言调整，面板 3/3 与 TypeScript/Vite 构建重新通过，未重复整套受控 L3。PM-A01–PM-A08 已完成本批自动验证，PM-M01 仍待人工体验。上述验证取证于提交前 dirty 工作区；后续提交与 PR 以 Git 事实为准。安装/保留运行环境未修改。


## 2026-10-21 提交前主干集成

用户授权提交、推送和提 PR 后，发现主干已增加 #370 候选请求实例隔离与 #371 事务内 Runner name_prefix 冻结。本批 rebase 至 `070bb7d7`，只手工合并两处并列文档说明，保留主干 freezeNumber/最终 submit 固定行为与本批执行快照事务；实施提交为 `33cd721f`。前述受控/live 仍明确归属 `9e02854e` 的实施工作区，本次未重复 live 或声称其覆盖新的主干 Runner 命名实现。

- Web 集成回归：`npm --prefix web test -- project-maintenance.test.tsx plan-tests.test.tsx use-candidate-starts.test.tsx candidate-starts.test.tsx`，4 文件 17 项通过；`npm --prefix web run build` 通过。
- Go 首轮：`C:\SteerlineDebug\pm-evidence-20261021-pr-integration/summary.json`，5 包 45 条记录、42 通过/3 失败（含父测试），4 个产品包全部通过；应用层失败限定为旧执行配置重启夹具：虚构 `owner/repo` 远端无法 fresh fetch。补本地隔离 bare remote、最小材料及显式核对后，`pr-preset-fix` 又揭示旧 `plan.md` 不符合现行 `.plan.md` 规范，该失败也保留，不回写为通过。
- 最小修正仅限测试：共用 stopped Run 准备函数采用 `task.plan.md`/`task.tests.json`、空清单与明确无需自动测试说明，Git transport rewrite 保持原仓库业务身份；显式沿项目 actor 核对，删除已失效的 ResolveBase 替身。原运行时替换、原 Run/input/停止及重启断言均保留；没有关闭远端或材料验证。
- 修正后失败项定向补验：`go run ./cmd/test-feedback -out C:/SteerlineDebug/pm-evidence-20261021-pr-preset-restart -- ./internal/app -run '^TestRunnerPresetFreeze/(reject_mismatched_run_after_explicit_restart|accept_replacement_after_explicit_restart)$' -count=1` 通过（182.463 秒）；共用夹具的 `TestStartupCancellationOutboxIsDurableBeforeRunnerStart` 另由 `pm-evidence-20261021-pr-outbox` 通过（93.263 秒）。对应 `summary.json`/JSONL 保留源码 dirty、原选择和逐项事实，未重跑已通过的全部场景。

最终文档检查 508 份 Markdown、3183 条本地导航链接、0 错误，差异空白检查通过。PM-M01 仍后置；安装环境未修改。提交与 PR 信息以 Git 事实为准。


### PR #373 后续主干集成

推送期间主干又合入 #372，因此继续 rebase 至 `9cf84463`。保留其 Runner 完整绑定/同快照观察和部署流程，仅合并测试 owner 并列条目及生命周期说明；产品维护实现未放宽。最终源码提交以 PR head 为准，前文 `33cd721f` 等为当时取证历史。

`pm-evidence-20261021-pr-main2/summary.json`：6 包 80 项中 79 通过，唯一失败为既有 HTTP 消费者依赖白名单未列 `projectconnection` 已使用的 `processwindow`。检查该包只设置本机窗口启动标志，不包含数据库、Runner 或业务 owner，故在原边界测试中仅按精确包名允许该通用工具，保留其余后端/SQLite 禁令。`pm-evidence-20261021-pr-consumer-boundary/summary.json` 定向补验通过（13.113 秒）；前轮失败单独保留。维护/生成/恢复、Runner 绑定/观察与 quick-owner 选择的其余 79 项均已在最新主干通过，未重复无变化的 Web 或 live。

本轮文档检查为 518 份 Markdown、3279 条本地链接、0 错误，差异空白检查通过。PR：[erranliu/SteerLine#373](https://github.com/erranliu/SteerLine/pull/373)。
