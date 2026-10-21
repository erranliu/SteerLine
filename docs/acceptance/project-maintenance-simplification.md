# 项目更新与候选版本收敛验收

专属范围：[Spec](../spec/project-maintenance-simplification.spec.md) · [Plan / 测试计划](../plans/project-maintenance-simplification.plan.md#测试计划) · [长期设计](../product-design/project-maintenance.md)。普通开发，无 roadmap Task ID。本文件仅归属本次收敛，不合并原 PM/TM 的结果。

2026-10-21 用户确认：每个新候选采用最新版本；Plan 更新自动生成测试安排；过期候选保留且不可关联当前测试；**过期候选 PR 合入仍可完成任务**。已实施，产品自动验收按下方实际记录核对；人工体验签收仍待安排。

最新 Runner 路径语义的后续适配、验证与旧构建限制处理见[本轮增量记录](#最新runner路径语义适配验证)。前轮记录保留其当时构建与结论，不作为当前传输契约。

## 证据登记规则

具体级别、组/场景、命令、环境、执行时点、通过条件及入口补齐见 Plan；T1–T5 为该表入口。产品执行时记录源码 commit/dirty、Runner EXE/version/SHA、规则及profile来源、独立包/证据根、实际选择/匹配用例、原请求/Run/材料摘要、退出码/完整性与清理。失败及补验分别登记；模拟不替代真实生成，缺工具/零匹配/skip/超时/缺证据不得记通过。

## 自动验收

### PMS-A01

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T2/T3。远端目标、本地分支、双侧数量、提交时间和检查时间分别正确；日志可分页查详情、PR多关联可展示，无PR/查询失败不影响Git更新。证据：focused/web/controlled报告、API/页面及Git/provider事实。

### PMS-A02

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T2/T3。固定目标的一键快进成功；脏目录、未跟踪冲突、错分支、领先/分叉、Git进行中、锁/身份/网络/认证/远端删除及并发变更有明确诊断且不损失文件；重放/丢回执不追赶新目标，候选工作区未改。证据：原命令、HEAD/refs/文件前后、回执与页面。

### PMS-A03

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T2/T3。文档无需逐项确认；各新候选独立冻结最新代码及全部材料，原号重放仍为原版本；不同base/契约允许，缺材料/路径/前置或CAS冲突零额外派发，Task身份/依赖不被改写。证据：材料/候选快照、公开submit及事务事实。

### PMS-A04

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T3/T4。缺结果或Plan/规范/清单变化自动生成、同输入复用，profile/原请求固定；严格ID/空结果校验；系统保存并直接给候选，零旁置JSON写入、审阅/应用/提交门槛。证据：输入摘要/原Generation/Run、结果定义、inline输入及仓库前后摘要。

### PMS-A05

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T2/T3。缺profile可一次性设置，失败只显式重试，unknown/重启只核对原请求；连续新版和旧迟到结果不串用，停止/资源义务及共享引用保持，任务关闭不再新建生成。证据：原请求计数、版本/状态、公开停止与清理。

### PMS-A06

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T2/T3。内容变化才过期、代码提交变化不误报；同一最新材料下多个候选独立关联测试，过期不发起新collect/晚到只存历史，未知不宣称当前通过；完整文档/测试差异和五名额/真实资源占用分离正确。证据：定义/报告/collect原请求、完整分页diff、API/截图。

### PMS-A07

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T3。过期候选的已核实merged PR可完成Task，即使最新测试生成中/失败；交付按该候选原契约。显式停止参与仍不可交付；新旧版本PR竞争只记录一次，后继释放与全部候选停止义务完整，已完成不重开。证据：精确PR、候选契约、唯一Delivery/图与停止清单。

### PMS-A08

状态：已实施，定向自动验证通过；具体覆盖与未验边界见本文件末节。T1/T2/T3。维护主界面只有更新流程及具体问题，任务/候选展示与API一致；跨项目/版本缓存授权隔离，GET零生成/写文件，旧人工确认/apply入口无运行旁路。证据：CLI/API、页面/trace及请求/文件计数。

### PMS-A09

状态：T3及T4通过，固定构建及清理证据见末节。T3/T4分别验收。扩展既有受控多版本Go/Web流程及一次真实自动生成到程序报告/API/浏览器消费；没有手工应用、伪造Git文件或Agent自述通过；真实/受控证据与资源清理分别完整。证据：两场景summary/scenario、原Run/报告/截图、停止与清理。

## 人工验收

### PMS-M01

状态：待安排。参与窗口：T1–T4自动项通过后；地点为全新独立调试包，非安装环境。检查用户能否理解远端/本地、提交/PR、快进失败原因、当前与过期候选、完整差异以及“旧版本PR仍可完成任务”。可后置，仅阻塞体验签收；不成为无关后继开发门槛。记录参与者、包/版本、时间、问题和截图。

## 实施前文档规划核对

以下保留2026-10-21实施前的纯文档记录，后续产品结果见“本次实施与验证记录”。调查源码 b9e5002d38da4722dc8457338e063ce56817fb48，文档为未提交修改：

- python scripts/check_doc_links.py：523个Markdown、3421条本地导航，0错误。
- git diff --check：通过；另按实际字节检查全部12个修改/新增文档，UTF-8无BOM、无行尾空白通过，覆盖尚未跟踪的三个新文件。
- Spec的9个PMS-A项、Plan逐项链接和专属验收9个标题一致；PMS-M01的窗口/阻塞范围已单列。长期设计与Worker节点/异常/验证矩阵标记为已确认待实施，原PM/TM证据未改写。

未执行产品测试、构建、模型生成或调试包；PMS-A01–PMS-A09及PMS-M01仍待实施/验证，不因上述静态检查标通过。

## 本次实施与验证记录（2026-10-21）

源码为 b9e5002d38da4722dc8457338e063ce56817fb48 上本次未提交修改；未部署或读写 C:/SteerlineInstall、C:/SteerlineRuntime。证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-pms-20261021-144532`。所有 Git/数据库单元现场均为临时隔离夹具；L3 包均由唯一 new-debug-release 生成，不复制安装数据/配置/认证。

实施：schema 27 删除任务唯一快照、人工批准和文件应用权威，增加最新材料、生成方案/观察及原快进事实；候选独立保存完整契约/base/材料摘要/inline系统测试快照。TaskContract/FileReference 纯结构抽至 taskcontract，Task业务规则留在task/model，架构守卫继续禁止Runner事实层依赖Task业务。复用原 generate/1、原请求/停止/证据收集和唯一交付事务。Web收敛到检查、快进、日志、自动文档及过期历史/差异；旧生成/apply API/CLI入口删除。

### 已执行入口

- T1：`focused-final/summary.json` 为9包45项通过、complete=true（80.707秒）；最终含Gitea/纯契约及引用收口的 `focused-verified-2/summary.json` 为9包51项通过、complete=true（63.492秒）。`preset-corrected/summary.json` 为原宿主换代专项通过（181.290秒），`slots-verified/summary.json` 为最新版本五名额/七版历史及原号重放通过。实际枚举和输出各存 summary/JSONL，不以零命中计覆盖。
- T2：`web-final/vitest.log` 为8文件72项通过；`web-final/documents.log` 为候选完整前后文/任务最新文档2项通过，共74项。`web-final/build.log` 为TypeScript及生产Vite构建通过。
- 架构/入口：`boundary-final.log` 记录 internal/testkit/boundary、cmd/test-feedback 全部通过；E2E live/runtime 标签编译通过（仅编译，不记场景通过）。
- T3：`controlled-5/summary.json` passed=true、coverage=selected-L3；包 `C:/SteerlineDebug/dev-20261021-151228-b9e5002d-dirty`。实际 Go/Web/Edge 场景1项通过（浏览器3.5分钟）；三版Plan、同新版两个候选、仅代码变化复用、一键FF、完整前后文、旧PR完成和双项目隔离均有断言。包内 `evidence/project-maintenance/proof.json` 的 passed/stopped/resources_settled 均true；reset.json保留任务/契约且清除材料和执行。浏览器facts.json保存三个候选和各版来源；retained-candidate.png已查看，过期保留、历史测试禁用及PR完成提示可见。
- T4：`live-3/summary.json` passed=true、coverage=selected-L3、cleanup_complete=true；包 `C:/SteerlineDebug/dev-20261021-152957-b9e5002d-dirty`，完整真实生成→inline候选快照→实际通过/失败程序→API/浏览器链通过。见下方固定构建/原Run/停止与远端清理。

### 失败及修正

- `controlled/summary.json`：生成包Web编译遇到测试工具不支持的exact属性；已改选择器。`controlled-2/summary.json`：生成期间编辑驱动导致构建失败；之后固定源码再生成。两包未开始产品场景。
- `controlled-3/summary.json`、`controlled-4/summary.json`：首个候选和材料流程通过，第二候选分别暴露驱动缺start_candidate入口和图版本参数；已沿原正式application client补齐。两次包proof均记录业务停止/资源结算，未记场景通过。
- `focused-complete/summary.json`：新增宿主换代专项时发现旧夹具生成结果缺PlanText，严格空安排检查正确拒绝；补全原Plan、构建和停止来源后 `preset-corrected`通过，未放宽产品校验。
- 最初架构检查拒绝runnerstate/projectmaintenance模型依赖task/model；改为共享纯契约声明，仍禁止Task业务传递依赖，`boundary-final.log`通过。最初文档链接检查发现已删除apply.go旧链接；改为固定原commit历史链接，`doc-links-final.log`记录523个Markdown、3394条本地导航、0错误。

### 覆盖与未验边界

PMS-A01/A02：真实隔离Git覆盖ahead/behind/diverged、提交时间/标题/日志、脏/未跟踪、错分支、游离HEAD、更新锁、进行中merge、固定原号FF。Gitea关联/无关联/服务错误/坏响应/外仓响应用只读HTTP夹具；接口来源为[Gitea官方提交关联PR文档](https://docs.gitea.com/api/1.25/operations/repo-get-commit-pull-request/)。GitHub用已配置命令provider的现有gh认证。未在真实provider人为制造认证故障或Git并发写入；不宣称穷举OS/Git故障组合。

PMS-A03–A06：TestPMSGenerationFreshnessAndCandidateVersions、TestPMSGenerationUnknownDoesNotOfferAutomaticRetry、TestPMSMaterialInputsDoNotRequireArrangement、TestPMSCandidateSnapshotCASVersionsAndReplay、TestPMSExpiredEvidenceNeverStartsNewCollection，配合TestTM原请求/取消/证据来源回归及T2/T3。新旧候选快照、未就绪零预留、原请求不升级、同输入复用、profile缺失、unknown零重发、过期停collect和保留历史、五名额均有定向证据。

PMS-A07：TestRunnerTaskDeliveryWithoutWorkflowReady覆盖不同契约候选、原PR合并、事务回滚/唯一交付、改身份拒绝、原退出事实及全候选停止、后继就绪；T3实际将第三个已过期候选的PR合入并核对该候选交付。未再复制一次真实多PR竞争矩阵。

PMS-A08：TestPM/TestTM及HTTP/CLI定向回归、T2/T3核对旧人工入口移除、GET只读、命令信封和跨项目缓存/业务隔离。PMS-A09的T3与T4均已通过。PMS-M01继续待用户体验签收，不记人工通过。

- `focused-verified/summary.json`：删除旧旁置文件引用后，一处旧SQL夹具仍将.tests.json当合法引用；改为精确inline快照并新增旧文件/篡改快照拒绝反例，最终 `focused-verified-2`通过。没有恢复兼容路径。
- `live/summary.json`：最初误选旧包中的caller-fixture-v1为真实EXE，未计真实通过。该失败包 `dev-20261021-151847-b9e5002d-dirty` 没有Candidate/Run，原proof仍失败；`post-stop-runner.json`及原shutdown证明服务停止、零Run/check资源、clean=true，`postflight-cleanup.json`记录核对零PR后按精确lease删除本包base。原生成无Run绑定的caller未核实记录保留，未重置/伪造停止投影。驱动已在任何推送/启动前拒绝fixture版本。
- `live-2/summary.json`：固定干净13ef123真实generate完成（正式codex.agentMessage.final_answer），后续开发submit被旧EXE明确拒绝：该构建不认识现有NamePrefix字段。候选原请求不重发；包 `dev-20261021-152334-b9e5002d-dirty` 原proof的stopped/cleanup_complete均true。
- 最终补验采用干净Runner `017e41071f645b4dc2e448ce8151fe7dfa8461d7`（包含候选前缀与inline材料冻结），从Git archive独立临时源码构建，不修改Runner仓库/默认EXE；版本0.2.0-pms-017e410。新增system-snapshot-binding.json按真实Runner公开Material摘要核对系统快照字节，不能把忽略inline正文的构建当作已经交付材料。此项只证明指定构建；采用其他文档路径语义的Runner必须重新适配/核对，不能据本次结果宣称通过。

### 最终真实结果与收尾

`live-3/summary.json` 整场通过。实际Runner `017e41071f645b4dc2e448ce8151fe7dfa8461d7`、source_dirty=false、version `0.2.0-pms-017e410`、SHA256 `80ebc805dfe620f4b1387381b50369420a8a8fd935f073352b5da00f816b4799`、resource digest `2bcf605cf715bf1e7c9456198fe935fd897222dfe19edd6a8749512a50baa37e`。独立archive源码/构建manifest位于 `%TEMP%/steerline-pms-real-runner-path.txt` 指向EXE的相邻目录，不修改Runner工作区或原默认EXE。

- 原生成Run `447d017e955e9e9b54bc33ffbde8114d`，原开发Run `8d5bb31ba1e3d2c1b0c0dc55fb502416`，两者final来源均 `codex.agentMessage.final_answer`；实际本机Codex profile为gpt-6-luna/medium，生成显式read_only。系统Generation `d3f669669cf80920ef68f28bdc20083f` 的输入、结果及候选inline快照经公开Runner Material摘要逐字节核对，`system-snapshot-binding.json` passed=true；没有应用或提交安排文件。
- 被测commit `494d098a26f954ef10e7053c1e126f90db807bf5`；真实通过报告 `2e9bb5a05f5c42a6891582adeca78c36`、失败报告 `be13b49261d742d78b013966dd66e888`，程序退出0/1、前后同commit且clean。API依次1/0/0、0/1/0并保留两次历史；两轮实际Edge均通过，pass/fail-api.json、程序日志、browser日志/trace/截图保留。已查看失败页截图，当前失败、原证据、历史2条可见，不以Agent自述成功替代程序结果。
- `proof.json` 的browser_passed/browser_failed/stopped/cleanup_complete均true。`verify-stop.log` 核对原host、固定EXE及两原Run/input_digest，各自及聚合resources_settled=true。`owned-refs-cleanup.json` passed=true，唯一base `codex/debug/dev-20261021-152957-b9e5002d-dirty/base` 和原head `codex/runner-8d5bb31ba1e3d2c1b0c0dc55fb502416` 均核对commit/无PR后按精确lease清理，远端复查为空。未创建真实PR；旧PR交付由T3受控真实Go/Web链验证。
- 收尾补验：`gitea-verified/summary.json`通过，额外覆盖404时核对仓库访问权限，区分真实无关联与无访问权限；`ff-verified/summary.json`通过（38.273秒），覆盖 `--no-overwrite-ignore` 下冲突保留原HEAD/本地忽略文件字节、人工移走冲突后成功快进。除本次变更相关补验外未重复Full或其他场景。

前轮结论（当前传输契约由下节取代）：PMS-A01–PMS-A09在上述定向范围通过。PMS-M01仍待安排，仅体验签收未完成。当前schema27不兼容旧开发库；本次未部署、未使用/读写/重置安装环境。当时发布需使用与上述inline冻结材料契约相符的固定Runner；仅工作区路径的新Runner语义尚未适配，不能将wire字段被接受当作材料已传入。

## 最新Runner路径语义适配验证

2026-10-21继续实施，源码仍为 b9e5002d38da4722dc8457338e063ce56817fb48 上未提交修改。独立证据根 `C:/Users/lmas1/AppData/Local/Temp/steerline-runner-paths-20261021-154222`；以下路径相对此根。最新 Runner 远端 main 与本地干净源码均核对为 `4d11148165ca36e4400df46e6bdadc30812fee25`，从当前源码独立输出 EXE，不使用旧构建或修改 Runner 工作区/默认 EXE。版本 `0.2.0-paths-4d11148`、source_dirty=false、SHA256 `c8bab5205b97ba1ea50bedc86fdd96f6534857f30f6a3a2d6a4e9b5c04c72ee5`、resource digest `7b9f8f744d76aa5762ccd2da520298b4ace72fdd12dc64cb70107ba5aebd13cb`；构建来源及公开 offline capabilities 保存在 runner/。

实现：开发文档仅为 worktree 相对路径，系统安排与测试说明冻结到公开实施/代码审计执行提示，保留发布模板和用户覆盖正文；generate/1 继续使用公开 inline 输入。删除虚拟系统文档引用及旧复制摘要要求；配置 profile 摘要与实际请求摘要分开，原请求重放不再组合提示。缺必要公开模板拒绝新候选，没有旧构建回退。Spec、Plan、长期设计和 Worker 既有矩阵同步说明本次传输差异。

### 定向自动验证

- `focused-final/summary.json`：4包86项、passed=true、complete=true，13.029秒，覆盖 runnerclient/taskrunner/SQLite/testmanagement 的 PMS/TM/Runner 定向用例。`public-reservation/summary.json`：1包3项、complete=true、通过，6.406秒，包含最新真实 EXE 的公开 run validate、路径与保存契约区分、profile 摘要及原请求重放。
- `prompt-verified/summary.json`：2包4项、complete=true、通过，8.648秒。使用最新公开 workflow 元数据，核对普通及审计执行提示、自定义正文/报告/发布指令保留、缺模板拒绝及系统快照绑定。
- app owner 在 `verified/summary.json` 中单包通过（225.422秒），覆盖自动材料/生成及六种 preset 情况；该报告整体失败仅因当时 SQLite 测试仍断言旧 wire commit，不能将整份报告记通过。修正为路径与 SteerLine 自有契约分别核对后，以上 public-reservation 与 focused-final 均通过。
- `boundary.log`：架构边界及 cmd/test-feedback 选择守卫通过。`compile-current.log`：最新驱动 live/runtime 标签编译通过，仅计编译检查，不计产品场景通过。

### 失败与补验

- 最初 `focused/summary.json` 的 app 夹具没有声明执行模板，被新准入正确拒绝；为测试 Runner 的公开 capabilities 补齐其自身模板。未放宽产品准入。`verified/summary.json` 的旧 commit 断言如上修正。
- `live/summary.json` 仅构建失败：驱动局部变量遮蔽 context 包，未启动产品场景，已修正。
- `live-2/summary.json` 整场失败、cleanup_complete=true；包 `C:/SteerlineDebug/dev-20261021-155150-b9e5002d-dirty`。最新真实生成与原候选、worktree 材料及提示快照核对已通过；旧驱动不识别最新 waiting_input/result_committed 边界。按原 Run/input 摘要正式停止候选后，程序通过/API读取成功，但浏览器恰遇新包构建临时替换源依赖而缺 playwright，未记浏览器通过。原停止请求/核对保留在 postflight-*；包 proof 的 stopped/cleanup_complete 均true，专属 refs 已核对并清理。驱动已适配公开等待输入边界，并将原 Run 停止移至失败路径也执行的收尾；新包独立重验，不复用失败现场。

### 最新构建真实结果与收尾

`live-3/summary.json` passed=true、coverage=selected-L3、cleanup_complete=true，实际执行323.452秒。全新 live 包 `C:/SteerlineDebug/dev-20261021-155831-b9e5002d-dirty`，来自唯一 new-debug-release；包 manifest 的 Runner source/version/SHA/resource digest 与本节固定最新构建一致。

- 原生成 Run `0868b384dc3216120202526ba5a4b324`、原候选 Run `e6d501c235f782907ee79b14ccac067d`；两者均为实际本机 Codex gpt-6-luna/medium，final verified=true、来源 codex.agentMessage.final_answer，生成显式 read_only。原候选如实结束于外部验收边界，未宣称实施/任务成功，未创建真实 PR。
- 包内 `system-snapshot-binding.json` passed=true。系统快照 SHA256 `d44adb05cb87fc7d4c92c42b2a814db3841d316b0c1c355bfb9a3851be64557d`，逐字核对公开 Run.workflow 的冻结执行提示。四项 Material 均为实际 worktree 路径，execution_location 等于原路径、commit 等于被测 base，digest/snapshot_path 为空；工作区没有虚构系统安排文件。
- 被测 commit `aab07104d5ca674dbdd9cdff2b31dfdc2f91468e`；真实通过报告 `73bef9490be348f1b6fabe5973408f27`、失败报告 `45c3c3f828a44a61813bf2d0478ff334`。程序实际退出0/1、前后同commit且clean。API先1通过/0失败，后0通过/1失败并保留两次历史；两轮真实 Edge 分别通过，原程序日志、API、浏览器 trace/截图保留。已查看失败页截图，显示原候选测试失败与历史2条。
- `proof.json` 的 browser_passed/browser_failed/stopped/cleanup_complete 均true。`native-stop.json`/`native-stopped.json` 核对原 Run/input；`verify-stop.log` 核对原 host、固定 EXE 及两原 Run/input_digest，各自及聚合 resources_settled=true。`owned-refs-cleanup.json` passed=true：仅本包 base `codex/debug/dev-20261021-155831-b9e5002d-dirty/base` 和 head `codex/runner-e6d501c235f782907ee79b14ccac067d`，核对 commit/无PR后按精确 lease 清理并复查远端为空。

本轮 PMS-A03/A04/A06/A08/A09 的路径适配增量在上述最小范围通过，前轮最新Runner未适配限制已解决。未重复Full、其他L2/L3或多版本受控矩阵；审计提示以最新公开模板L1验证，未额外启动审计模型。PMS-M01仍待人工体验签收。本次未部署、未使用/读写/重置 `C:/SteerlineInstall` 或 `C:/SteerlineRuntime`，未改 Runner 仓库/默认 EXE。

最终静态核对：`links-final.log` 记录523个Markdown、3396条本地导航、0错误；`diff-final.log` 的 git diff --check 通过；`bytes-current.log` 覆盖116个修改/新增文件，UTF-8无BOM及行尾空白0错误。

### 提交前主干集成验证

用户授权提交、推送与创建PR后，本次实现固定为 `00706c2f`，合并主干 `9095fd991ea0cfb3070446bd89217696787a94bb`（执行配置页面及首次启动空列表修复）。冲突仅在 quick-owner 登记、纯包边界和场景清单：保留主干执行配置测试/纯包与新增场景，同时采用本次自动材料/候选版本规则，删除已被本次替换的旧人工确认用例登记；任务详情自动合并后保留空列表保护及过期候选名额规则。

本节证据位于同一最新Runner证据根的 `publish-main-integration/`：

- `boundary.log`：`go test ./internal/testkit/boundary ./cmd/test-feedback -count=1` 两包通过。
- `app/summary.json`：`TestWebExecutionConfigLoadedSnapshot` 与 `TestPMSGenerationFreshnessAndCandidateVersions` 两项通过、complete=true，24.990秒；验证主干执行配置与自动材料流程接线。
- `web-tests.log`：任务详情、候选启动、启动状态、基线维护、候选文档、计划测试、执行配置7文件40项通过；`web-build.log`：TypeScript及Vite生产构建通过。
- `doc-links.log`：527个Markdown、3443条本地导航、0错误；提交前暂存区diff检查通过。

未重跑已通过的真实模型/L3或全量测试：合并未改动已验的Runner文档/提示传输规则；本轮新增风险由上述集成补验覆盖。PMS-M01仍待人工体验签收。提交、推送及创建PR仅操作本开发仓库与GitHub，不部署或改动安装环境。
