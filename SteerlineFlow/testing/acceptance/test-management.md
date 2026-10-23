# Runner generate/1 对接与最简测试管理验收

## 2026-10-20 合入修复后的固定主干复验

**generate 接管阻断已解除。** 本轮先将工作分支 rebase 到 SteerLine 主干 `f139e4a9`，确认 PR #365 和后续 #366 已合并；Runner 修复随 [PR #37](https://github.com/erranliu/SteerRunner/pull/37) 合入，最新干净主干为 `13ef12312944317e7b78d4de44977b92e65e70c2`（#39）。本轮独立实跑，未将之前 dirty A/B 构建的资格改记到新构建。精选公开原响应及命令见[复验证据](evidence/generate-takeover-recheck-20261020/README.md)。

- 源码核对：freeze 和 takeover 共用 workflow 的 `RequiresResultAcquisition`；内容流程的合法空配置不再误拒绝，实际 parser 意图仍校验冻结配置及解析契约。材料、workflow/version、codec、原 owner 与输入绑定检查保留；没有补写默认配置、迁移或公开字段变化。
- 本轮从 Runner 干净主干构建 `0.2.0-dev.3-r2`，SHA256 `482aafcffd8b61516f26e842361602741643f52e2d77d2be5ccea6461da4292f`，resource digest `a48bfb1aa7250009b21b4170532d877be01c23375ac386cb39e269794263ef91`，source_dirty=false。
- 原 k 包只读复验：原 generate `f6942435176c8f5d5430b15591e9ee8f` 和 development `8402cc9208fbecdd4164d59e0fedc47d` 均 compatible，聚合 true；revision、input_digest、request_id/request_digest 与原失败响应逐项一致。原 namespace 的 181 个文件路径/长度/SHA256 前后一致，服务前后均 not_running。没有启动或修改原现场。
- 新隔离 Runner 夹具 `C:/srt/gt-23720053/state`：真实 Codex 单回合 generate Run `5c6669c185ca96aab43eeaadf296eeee` 成功，原 final verified；显式原 Run stop → 原 service stop → verify-stopped（逐 Run 及聚合 resources_settled=true）→ 同 EXE 离线 verify-takeover（compatible=true）。原 request/input/revision 绑定一致，接管前后原数据库字节不变。非零命中 `TestSRContentGenerateTakeoverNative` 一项，30.25 秒，退出 0。
- SteerLine 七包定向 33 项（含子用例）通过、complete=true、零失败/skip，69.822 秒，覆盖生成取消/未知/换代、原停止 outbox、接管绑定及 CLI 预检。Runner application/resultacquisition 定向 27 项（含子用例）通过，覆盖正式 freeze、generate/conversation、材料/codec/解析契约反例；workflow 包本次筛选未命中，不计其整包通过。

完整原始输出位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-generate-recheck-20261020-1792476800182`。本轮只补固定主干的同构建与旧记录资格证明；此前上层真实 A/B、报告/API/浏览器和 i/j 清理继续采用下节各自固定版本证据，没有重跑整场或扩大为活动回合恢复资格。未运行 Full、前端、真实 conversation 或安装部署，TM-M01 仍后置。PR #365 的初始阻断描述同步标为历史状态，并链接本轮后续 PR。


## 2026-10-20 自动缺口继续处理

**最终结果：TM-A04 的真实跨构建接管及 TM-A08 同包完整闭环已验收，原 i/j 资源与分支遗留已清理。** 以下中间失败和历史阻断只保留其当时状态，不是当前待办。用户已授权两仓提交、提 PR 并检查后合并；本轮[提交前定向检查及关键证据](evidence/merged-pr-regression-20261020/README.md)随仓库保存，未部署；TM-M01 主观体验仍待用户安排。

最终包 `C:/SteerlineDebug/dev-20261020-133842-5612f97d-dirty`，SteerLine `5612f97d` 加本轮驱动修正；Runner `c122d641` 加内容接管修复，A/B 为 `0.2.0-content-takeover-a/b`，SHA 分别 `066a182f452ba0e80e51f5091bb5ab6baf31a6cf9085ed7e4cf7ff520234346a` / `ea8ad087931f55fed5da086503b76d01944fe3cbc88c191caae3e481bf901d6d`。两仓均 dirty，源码补丁、驱动文件原文/摘要、固定构建 manifest 留在证据根，不冒称干净提交。

- 原生成请求 `6d5327f82cda4b9eadd729fb65e9cf4f` / Run `784ed608d78a7e5784c98685e52c7a70` 经 B 实际接管。`generation-takeover.json` 记录不同 SHA/服务实例、原 Run/input/final/content_result 不变；`generation-after-takeover.json` 确认原请求仍 saved、原生成结果和 Run 不变。仅更新本包 Runner 传输 pin，无重新 generate/submit/reply/resume。
- B 上原开发 Run `5334c89977c5109cfbee470bf60e9c86`，代码 commit `cf101768ca7db7acfea3b9d558105750153222c2`；实际程序通过报告 `35448c3876f1427d88c2069a324c4bc0` 与失败报告 `2b8b72bb1aae4aa585bc9d2671418725`，原程序退出 0/1、前后同 commit/clean。两个 API 分别 1/0/0 与 0/1/0、历史两次；两轮 Playwright 分别 4.2s / 2.3s，通过。
- `proof.json` 的 browser_passed/browser_failed/stopped/cleanup_complete 均 true；`verify-stop.log` 按两个原 Run/input_digest 确认全局和逐 Run resources_settled=true。整包已停，不以 PID 消失代替证明。
- 补充远端核对发现 native publish 曾推送原 head，但因“base/head 没有差异”未创建 PR；原程序仍核实代码 commit 未变。原驱动只清 base，已修为正式资源结算后核对原 Run head/base、精确 commit/lease 及无 PR，再删除并复查。本次同包后验 `owned-head-cleanup/summary.json` 实跑 1 项通过，24.496s，包内 `evidence/test-management-owned-cleanup/` 保留新 verify-stop 和 `owned-refs-cleanup.json`；未改原 proof，未重跑模型。两条本包远端分支最终均为空、PR 数为 0。
- 独立后验脚本 `verify-closeout-utf8.ps1` 核对同一批生成、A/B、报告/API/浏览器、两原 Run 结算、实际 Runner SHA 和远端事实，`qualification.json` passed=true。初版 PowerShell 5.1 验证脚本未显式 UTF-8，读取中文原结果失败，改为 UTF-8 后对同一原证据重验；无业务重试或证据改写。

以上原始日志与后验均在 `C:/Users/lmas1/AppData/Local/Temp/steerline-auto-closeout-20261020-131948/`。主场景包装退出 0；完整闭环和后验分别记录，不据此声明完整 L2/L3 或其他原生恢复矩阵通过。

本节更新下文的 Runner 阻断和 i/j 清理未知结论，历史记录不覆盖。Runner `c122d641` 加本轮工作树修复已确认内容流程 parser 配置不适用的误校验，修正原本错误初始化的内容夹具，并验证生产 admission、queued/result、原材料/codec 及开发配置拒绝。四包定向与三个内容受控场景均通过；临时 A/B 固定构建和原 k 包只读 compatible 证明见 `C:/Users/lmas1/AppData/Local/Temp/steerline-auto-closeout-20261020-131948/`。本仓增加 `scripts/test-test-management.ps1 -RunnerCandidate <B>` 变体，沿现有统一新包与真实测试管理链验证 TM-A04，原模型/权限/材料保持，只有本包 Runner 传输 pin 经正式预检/verify-restart 后更新。

原 i/j 两包资源与专属 base 已完成自动收尾：`dev-20261020-110841-0752537e-dirty` 的 Run `3dd1c855f6ceabc807cbc4fa95b1e893` 与 `dev-20261020-111523-0752537e-dirty` 的 Run `71d386a024a728c0e32ce3e14734de5b` 均在 B 启动 workers 前提交原 Run/revision 的持久停止信封，之后只按原请求查询，stop_status=stopped。新服务正式停服、逐 Run 与全局资源均已核实；原 caller 的失败 shutdown 与原 proof 保持失败。核对没有本包 PR、base 仍是原 commit 后以精确 lease 删除并复查远端为空。证据根下对应两个包目录的 `summary.json` / `final-service-status.json` / `final-verification.json` 和原请求 JSON 保存全部绑定。

清理驱动的中间错误也保留：一次 service stop 使用不适用的顶层 expected-instance 参数，被明确拒绝后沿原 request/body 修正；一次过早将 stopping 期间的空 discovery 当作实例替换。最后均以原 service stop ID/instance 的 last_shutdown 和正式 verify-stopped 核实，不重发业务输入，不用进程消失代替结算。

首个新包 `dev-20261020-132406-5612f97d-dirty` 真实生成、程序通过/失败、API/浏览器及完整结算均通过，但可选 B 参数被 debug prepare 的环境隔离清理，未执行换构建；脚本按缺少 generation-takeover.json 失败，不计 TM-A04 通过。已在 prepare 前固定该参数，与既有 source 参数同样处理；补验使用新包，不修改该包冻结材料。最终换构建结果待下方追加。

驱动后续修正与失败现场：`132920` 包因参数名与业务 candidate 变量重名而编译失败，无原生运行；`133140` 包通过 A/B 预检与 verify-restart，但过早把启动返回当 B ready，失败后正式公开证明确认原生成资源已结算，caller 未核实启动记录保留。补上有界 ready 等待并定向通过后，`133424` 包已完成 B 实际接管、原生成结果 API 核验；后续 start_task 出现 Windows `request_receipt_persist_failed`。原请求记录/唯一 ID 保留，正常 stop/verify-stop 及 base 清理均通过，整场仍失败。驱动现对该公开未知回执错误只查询原 request_id，不重新提交；原输入先落盘，已知拒绝直接保留失败。`steerline-control-reconcile-directed/summary.json` 的 19 项全部通过、零 skip，14.777s；接管、就绪及早期编译失败与最终补验分别留档。新的完整场景继续使用新包。

2026-10-20 合并后补验：相关三个 Web 文件 26 项与生产构建通过；固定真实 Runner 的 `TestEnvironmentCheckPublishedContract` 1 项通过、零 skip（此前缺 EXE 的跳过已补）。证据根 `C:/Users/lmas1/AppData/Local/Temp/steerline-merged-acceptance-20261020-125058/`，分别为 `web-tests-final.log` / `web-build.log` / `published-contract/summary.json`。从干净 Runner `c122d641` 构建的 `0.2.0-acceptance-20261020` / SHA `d94760b643a4ce8469edf3c3a4494b370be6160dc0aa8aa9c046474761a92a1f` 只读核对原 k 包，已完成 generate 仍 unsupported，原开发 Run compatible；`runner/tm-k-takeover.json` 保留完整封套。TM-A04 继续阻断；不重启/重置 i/j/k、不继承新版本通过。TM-M01 的历史截图/trace 和后续可交互窗口说明见[人工操作](merged-pr-regression-20261020.md#执行页与测试结果可读性)。本页下文的固定 L1 失败保留历史，当前补验结果另见该导航。

日期：2026-10-19；2026-10-20 更新实现与真实验证。仅归属[本任务 Spec](../../spec/test-management.spec.md) / [Plan](../../plans/test-management.plan.md)，普通开发，无 Task ID，不进入 roadmap。SteerLine 侧实现、真实生成、实际通过/失败→API/浏览器、跨构建接管及原 Run/整包资源结算均已验收；当前结果见本页开头，历史失败不改写。人工 TM-M01 后置，不继承其他任务的原生资格。

## 跨仓前置

| 前置 | 交接内容 | 当前状态 |
| --- | --- | --- |
| R1 | generate/1 的正式公开能力/schema、只读配置、结果与停止语义 | 已固定 PR #33 合并提交 `9586ac4dee53882ba4349710e94c950cf47fd8db`，实际公开 schema 正反验证通过；不等待 conversation 产品能力 |
| R2 | 原开发 Run 的不可变报告/日志公开收集和读取、当前 commit/clean 观察 | 已由 Runner PR #34 合入，当前主干 bf3ec58 保留；正式公开能力和本轮受控 CLI 验证已核实。本仓已接公开客户端、持久收集/读取与当前性投影；本轮结果见末节 |
| R3 | 同构建 EXE version/SHA、commit、resource digest/schema、定向证据 | 当前另有干净 Runner 13ef123 固定构建的同 EXE 真机及旧记录只读复验，见页首；A/B 报告/浏览器/结算保留原 dirty 版本证据，不串用资格。历史版本仅证明各自当时范围 |

Runner 实施固定构建：`9586ac4dee53882ba4349710e94c950cf47fd8db`、source_dirty=false、version `0.2.0-dev.2`、windows/amd64、SHA256 `33851d039193ca1f2f6766ab15e55abb54bda118ed673f81f45ad2cc80ec6b49`、resource digest `536edcaf2ed199109a1b42875f61fbb27af048a6441c5494d8afe36f4ca1bf56`。干净源码经 Runner 自身 build.ps1 输出到 `%TEMP%/steerline-test-management-runner-9586ac4/`；该目录含 EXE、build.json 和实际 capabilities.json，本仓 `internal/runnerclient/testdata/content-capabilities.json` 保留测试所需公开子集。未使用 Runner 工作区原有旧 EXE。

## 自动验收

命令、环境、时点和能力补齐安排统一见[Plan 测试计划](../../plans/test-management.plan.md#2-测试计划)。下表只登记实际结果与证据，不替代通过条件。

| ID | 验收内容 | 状态 | 证据 / 实际版本 / 限制 |
| --- | --- | --- | --- |
| TM-A01 | generate 客户端契约、只读配置映射、开发隔离、缺能力零回退 | 受控验证通过 | 实际公开 schema fixture 正反例、配置白名单映射及缺能力拒绝；不建设 conversation 操作接口 |
| TM-A02 | 清单/安排映射、PlanRef 同提交推导、固定快照和空/缺/无效语义 | 受控验证通过 | 严格 JSON/ID/路径、隔离 Git 同 commit 读取、已有 E2E 登记对齐；沿原候选事务固定，不增加 Task 字段或门禁 |
| TM-A03 | 原生成请求、固定材料、空结果保护、条件保存及显式继续保存零重发 | 受控验证通过 | 内存公开 Runner fixture + SQLite CAS；真实生成保存另记，不把此项当 live 闭环 |
| TM-A04 | 未知/取消/迟到/重启、持久化先于提交、无 Task 副作用与停服结算 | 已验收：受控与真实跨构建接管 | 原生成请求/Run/input/final/result 跨 A/B 保持，正式重启与最终两原 Run 结算通过；干净主干同 EXE/原 k 包复验亦通过，版本与证据见页首 |
| TM-A05 | 实际报告/公开产物回传，环境/选择/日志及干净同 commit 限定 | 功能链通过 | 定向验证及同一原开发 Run 上真实程序通过/失败，经公开 collect/list/read/workspace-git 到正式 API 与浏览器；原退出码 0/1，干净同 commit；整包结算另记 |
| TM-A06 | 来源/版本隔离，幂等冲突、顺序与补测历史 | 通过 | 不可变 CAS、字节摘要、原请求/来源与冲突顺序定向验证；真实第二次失败覆盖当前通过，历史保留两次和各自原证据 |
| TM-A07 | 正式 API、执行页汇总/详情/历史/日志/切换/离线 | 通过 | 前端 26 项、API 边界及真实浏览器通过/失败两轮；无新页面、执行按钮或浏览器写接口，见末节 |
| TM-A08 | 固定真实 Runner 生成保存，实际通过/失败报告到正式 API/浏览器闭环 | 已验收 | 当前 133842 包的 A/B 接管、程序/报告/API/浏览器、原 Run/整包结算及本包 base/head 清理通过；历史 k 包保留原适用版本 |

每次执行记录 SteerLine commit/工作树、Runner pin、场景选择、命令、输出目录、退出/通过/完整性、原 Run/Command/Turn/材料与代码来源、包和清理事实。失败和补测分别登记，不覆盖历史失败。模型成功、受控 fixture 或编译不能代替真实闭环。

## 人工验收

| ID | 内容 | 窗口与阻塞范围 | 状态 |
| --- | --- | --- | --- |
| TM-M01 | 执行页密度、中文文案、折叠、日志可读性 | 自动闭环后提供同一独立调试包，用户窗口待定；可后置，不阻塞无关开发 | 待安排 |

所有临时/真实调试先运行 `scripts/new-debug-release.ps1`，使用 `C:\SteerlineDebug` 下独立包与 SteerlineTest。禁止使用/修改/重置 `C:\SteerlineInstall` 和 `C:\SteerlineRuntime`，不复制其数据库、认证或配置。启动后冻结方案，换 Runner/方案生成新包。模拟显式 `--simulate`，不能计作 TM-A08。

## 本次文档工作

2026-10-19：建立一套 Spec/Plan/专属验收，合并 Runner 内容流程对接与最简测试管理范围。功能、模型、测试和部署未启动；仅进行仓库调查、文档链接与验收映射检查。提交/PR/合并未执行。

2026-10-19 文档审查修订：删除无人消费的 conversation 操作接入和新增 tests_ref，改为从固定 PlanRef 推导旁置 JSON；只复用 Codex 程序/模型配置并显式使用 read_only。首版仅认可干净同 commit，删除脏文件指纹要求；Runner 只提供通用报告/日志出口及 Git 观察，不承担测试业务。补齐空生成保护、缺失/无效安排不加门禁、查询与显式继续保存的区分，以及首批入口适配范围。全部功能验收仍为待执行。

## 实施与验证记录（2026-10-19）

源码为 SteerLine `4c5409f398585b9fd3623d625d4e2d0563efec13` 加本次工作树，未提交、未部署。已实现 generate/1 公开值/schema、只读白名单映射、持久原请求与保存/停止用例、原生 CLI、清单/安排校验、候选固定快照、test-feedback 报告输出、只读 API 与执行页。SQLite schema 24，不增加旧版本迁移。SQLite 仅依赖 `testmanagement/model` 纯记录，业务调用仍归 `testmanagement`。

受控证据：

- 初轮定向 `go run ./cmd/test-feedback -out <focused> -- ./internal/testmanagement ./internal/runnerclient ./internal/storage/sqlite ./internal/app ./internal/taskrunner ./internal/product/httpapi ./internal/product/applicationhttp ./cmd/steerline ./cmd/test-feedback ./tests/e2e/internal/driver -run '^TestTM' -count=1`：23 项通过、complete=true、零 skip；目录 `%TEMP%/steerline-tm-focused-20261019-1946/`。后补迟到取消、保存回执丢失和生产待验证查询用例，最终同命令证据为 `%TEMP%/steerline-tm-focused-final-20261019/`，最终结果以下方记录为准。taskrunner/applicationhttp 的已有回归由 L1 补足，不把零命中新测试前缀的包单独宣称新增场景通过。
- 首轮固定 L1 `%TEMP%/steerline-tm-l1-20261019-1940/` 失败：架构守卫指出 SQLite 直接依赖带 CLI 的测试业务包。已把记录移入纯 model 并纳入纯依赖守卫；原失败日志保留。原执行器当次将整体标记 incomplete，报告仍保留实际断言失败；随后修复 managed 输出优先保留已知断言失败，不改变主执行器退出码。
- 修复后 `go run ./cmd/test-feedback -level=L1 -out <new>`：passed=true，执行 156.3 秒，证据 `%TEMP%/steerline-tm-l1-20261019-1951/`。追加 `managed-l1.minimum.json` 实际输出 passed；本开发工作树 dirty，before_clean/after_clean 均 false，因此此报告按业务规则不能计成当前代码的已核实通过。
- Web 定向三个文件 `plan-tests.test.tsx / runs.test.tsx / queries.test.ts`：26 项通过；`npm --prefix web run build` 通过。覆盖失败/历史/空/缺/无效、离线和缓存隔离；尚未进行真实报告驱动的浏览器验收。
- 文档链接检查 486 个 Markdown、2993 个本地链接，0 错误；`git diff --check` 通过。后续文档收尾仍做链接检查。

最终定向反馈：`%TEMP%/steerline-tm-focused-final-20261019/summary.json` 为 passed/complete，26 项通过、零 skip，执行 11.286 秒；包括后补的取消后迟到结果不保存、保存完成但回执丢失时零重生成、生产查询缺 R2 不虚构通过。收尾文档链接检查仍为 0 错误，前端构建通过。两次探针包中当前均未发现活动产品进程；成功包以公开 verify-stop 证明结算，较早未核实包仍保留，不用进程不存在代替它的结算事实。

真实生成探针：先运行 `scripts/new-debug-release.ps1 -SteerRunnerExe <固定 9586ac4 EXE>` 生成 `C:/SteerlineDebug/dev-20261019-194127-4c5409f3-dirty`，使用包内 SteerlineTest 与一个最小测试选择输入。默认 live 冻结，复用外部既有登录，未复制安装认证/配置/数据库。通过仓库 `independent-test.ps1` 的独立同用户进程启动产品；未执行开发 Task、推送分支或创建 PR。

- 正式 CLI 请求 `bd5555b5d8d444099449a7f9a551f558`；Runner Run `f4e023e048c83b9b874c420623aea8fe`，`generate/1`，native outcome=succeeded、verified=true。Command `fffbffb676084915c410d9fdf11a321c`、Turn `01a153f9-7576-7880-abb1-1eba325951e2`。CLI 返回 state=saved、stop_status=stopped，文件为 `SteerlineFlow/plans/generate.tests.json`，内容 `{"tests":["fixture.clean"]}`。
- 证据目录为该包 `evidence/test-management/`：generate.json、generation.json、saved.tests.json、应用自有持久观察导出的 steerline-generation-record.json、stop.log 和 verify-stop.log。未读取 Runner 私有数据库/日志；保存记录中的原生来源来自运行时公开 inspect 观察。停服后额外尝试的 inspect --offline 不受此命令支持，runner-inspect.json 仅为该失败记录，不作为通过证据。
- 正式 `steerline stop` 成功；`verify-stop -runner-exe <包内固定 EXE>` 返回原 host/项目身份、该 Run/input_digest 及 resources_settled=true；shutdown clean=true、原 Runner stopped=true/resources_settled=true。两个产品服务已停止。
- 较早包 `C:/SteerlineDebug/dev-20261019-193620-4c5409f3-dirty` 的首次直接启动受到工具 kill-on-close Job 限制；随后独立进程尝试在项目 ready 前停止，留下公开 startup_unverified，未调用模型。已保留该包和原失败/未核实记录，不 reset、不删除 Runner 现场。新包独立启动、等待项目 ready 后完成上面的成功探针；不覆盖前一现场。

**仍未实现**：Runner R2 原 Run 报告/日志公开读取与当前 commit/clean；随后本仓报告收集持久化、来源去重和真实结果到 UI 的闭环。当前生产 `/runs/{candidate}/tests` 只投影固定安排与明确诊断，全部待验证。纯结果汇总和前端样例不能替代这部分。补 R2 不需要 Runner 理解测试 ID；只需通用不可变产物/内容/日志引用及当前 Git 观察，业务规则继续归 SteerLine。

## Rebase 与当前缺口审查（2026-10-20）

本节更新上一节 2026-10-19 的“等待 Runner”结论，不改写历史失败/未验记录。本次只按用户要求 rebase、检查和更新文档，未继续实现结果消费或部署。

### Rebase

SteerLine 原 detached HEAD `4c5409f3`，工作区为本任务未提交改动。建立 `codex/test-management` 分支，使用 include-untracked stash `6768545c7269352a35963f2f59f4d4c2934e3d0d` 保存后 rebase 到已 fetch 的 `origin/main`：`0752537ebb8e9c7fd34f673e32a3daf429f7fbb0`。恢复改动时解决四处冲突：CLI main 保留主干 restart/verify-restart 与本任务 tests 命令，验收索引/测试总入口/worker 文档保留两侧内容。31 个原未跟踪文件均存在，冲突索引为空；改动仍未提交，stash 备份保留。未 push、未创建 PR。

### Runner 状态与复测

- R2 实现提交 `7325a87`，PR #34 合并提交 `a259c3a`；当前本地 main 与 fetched origin/main 均为 PR #35 合并 `bf3ec581cb0b23a7358d95e5efa8ff5f607c2f55`，工作区干净。PR #35 的内容流程恢复增量没有删除 R2；本轮核对正式 capability 与实际 CLI。
- 当前公开命令为 artifacts discover/collect/list/read、workspace-git；Run 内固定目录 `.steerrunner/artifacts/`，收集为有 request_id 和 Run revision 的写操作，读取为 Run/artifact ID、base64、原字节 SHA256。单文件 1 MiB，每 Run 64 项/16 MiB；支持 host-local development，Orca environment 明确 unsupported。当前 Git 未核实时 verified=false、commit/clean=null；读取收集副本不依赖原工作区，仍须保留 state-dir 和运行服务。
- 仓库原 `bin/steerrunner.exe.build.json` 仍记旧 R1 `9586ac4`，本轮未拿它冒充 R2，也未覆盖它。从干净 bf3ec58 经 `scripts/test-foundation.ps1` 新建临时精确构建：version `0.2.0-dev.3-r2`，SHA256 `21ab747fdcb02a862402afa07baaae13fef0196f79f9e5e087a19ff8dfcce697`，resource digest `a48bfb1aa7250009b21b4170532d877be01c23375ac386cb39e269794263ef91`，source_dirty=false。
- `scripts/test-foundation.ps1 -Pattern '^TestSRRunEvidenceR2$' -OutDir <新临时目录>`：通过，命中 1 个顶层场景、exit_code=0、105.35 秒。真实 EXE/CLI/IPC/SQLite，原生端为受控 fixture，未调用真实 Agent；覆盖收集重读/原字节/新旧副本/跨 Run、非法路径/超限、当前 Git 变化、零新 Agent 输入及 Run 停止/原工作区清理后读取。证据 `%TEMP%/steerline-runner-r2-audit-20261020/` 的 result.json、integration.log、source-identity.json、steerrunner.exe.build.json、version.json、capabilities.json 与 r2-public-evidence.json。
- Runner `go test ./internal/contracts ./internal/resources ./internal/cli ./internal/application ./internal/execution/workspace -run '^TestR2' -count=1 -v` 通过：实际命中 resources/CLI/application/workspace 的 R2 用例；contracts 包没有此前缀，不把零命中算契约包全面验收。

### 本仓剩余缺口

| ID | 缺口及代码证据 | 最小收口方式 / 验收 |
| --- | --- | --- |
| G1 | `runnerclient/client.go` 尚不允许五个 R2 命令；catalog/runnercontract 没有 run_evidence/schema 或产物/Git DTO；`app/test_management.go` 的 PlanTests 仍把 nil reports 和未知 Git 传给 Project，所以生产通过数始终为 0 | 接公开客户端与能力校验，沿原候选观察路径消费，不新增 Agent 或测试触发。TM-A01/A05/A07 |
| G2 | `cmd/test-feedback/main.go` 默认输出系统临时目录；managed.go 只在 opts.out 追加报告；当前无 Run 内目录约定和产物忽略规则，Runner 不会替调用方扫描系统临时目录，也不自动修改 ignore | 用现有 -out 显式指定 `.steerrunner/artifacts/<执行ID>/` 新目录并由项目维护忽略；日志相对引用映射到同一执行产物，按大小上限收集必要证据。TM-A05 |
| G3 | SQLite 只有 test_generations/request，没有测试报告或 collect 原请求记录；Report/Project 目前只是纯解析/汇总，没有来源持久化、原请求未知处理、摘要核对或去重冲突消费 | 持久 collect 意图先于调用，按原候选/Run/artifact/执行 ID 保存不可变事实，核对字节与来源后汇总；同源幂等、错配/冲突拒绝、迟到旧通过不覆盖新失败，接真实 workspace-git 当前性。TM-A05/A06 |
| G4 | generation.go:observe 在处理原记录前严格要求 r.Runner == s.Build；主干 runner_takeover.go:prepareProjectRunnerStops 和 application_verify_stop.go 的换代核验只枚举 Candidate/Check，没有生成记录。兼容换构建后生成观察/显式停止会被拒绝，持久取消也未加入启动前 stop 清单 | 复用主干已存在的公开 takeover 证明和 startup stops，保留原 pin/request/Run/input_digest，不把新构建直接视为可信；补兼容/不兼容及启动前取消优先的定向验证。TM-A04 |
| G5 | plan-tests.tsx 能渲染样例计数/历史，但生产只拿占位 pending；log 只是字符串。拟议 testing.management-local-codex 不在 suites.json，专属浏览器驱动未实现。既往真实生成及本轮 Runner R2 独立测试均不是同一产品闭环 | 将已核验报告和稳定产物引用/摘要接入现有只读区域；按 Plan 补最小真实通过/失败→Runner→API→浏览器与正常停止场景，不增加测试页或运行按钮。TM-A07/A08 |

G1–G5 已同步回原 Spec/Plan，继续使用这一套文档；不再列 Runner R2 实现为阻塞，也不把缺口检查当作已实现。主干已有 Task/Check 的兼容换代通过，不等于新增的生成记录也通过。

### Rebase 后本仓验证

- `go run ./cmd/test-feedback -out <新临时目录> -- ./internal/testmanagement ./internal/runnerclient ./internal/storage/sqlite ./internal/app ./internal/taskrunner ./internal/product/httpapi ./internal/product/applicationhttp ./cmd/steerline ./cmd/test-feedback ./tests/e2e/internal/driver -run '^TestTM|^TestSharedRunnerCompatibleBuildUsesPublicPreflight$|^TestTakeoverKeepsFrozenCandidatesAndRequiresOriginalRequestDigest$|^TestEnvironmentCheckAfterTakeoverUsesCurrentTransportKeepsOriginalSelection$|^TestDeploymentRestartCandidateBindingsPreserveOriginalState$|^TestTakeoverPublicContractRejectsChangedIdentityAndUnsupportedState$' -count=1`：31 项 passed、complete=true、零失败/skip，36.438 秒；证据 `%TEMP%/steerline-tm-rebase-audit-20261020/summary.json` 与原始 JSONL。
- 前端 plan-tests/runs/queries 三个定向文件：26 项通过；`npm --prefix web run build` 通过。保留新主干最近活动展示与本任务区域的接线。
- 未执行 Full、完整 L2/L3/L4、真实模型、真实 PR、安装部署或安装目录调试；本轮 Runner 受控场景使用它已有的隔离夹具，不是 SteerLine TM-A08。文档/差异检查在本轮收尾登记。

收尾：493 个 Markdown、3018 个本地导航链接，0 错误；`git diff --check` 通过，无未解决冲突。未修改 Runner 源码、原 bin 或安装环境，只在新临时目录构建并使用原受控隔离夹具验证。


## 缺口实现与验证（2026-10-20，继续实施）

本节是继续实施后的记录，前面的审查表保留为当时基线。仍为 `codex/test-management` / `0752537e` 加本任务未提交工作树，未部署。

- G1/G3：共用 Runner 客户端接入五个 R2 命令及公开 capability/schema；原候选观察先持久 collect 请求，未知只查原请求，明确 stale_revision 才重新规划；核验 Run/artifact/路径/长度/base64/SHA256 后写入 schema 25 的不可变 `test_evidence`。查询结合近期 verified commit/clean 与固定定义汇总，不从 Agent final 推断测试通过。
- G2：沿用现有 `-out`，约定原 Run 的 `.steerrunner/artifacts/<唯一执行ID>/`；项目维护 ignore，程序原子发布 managed 报告作为完成标记，首版只收集报告及同目录原始 summary。模板提供实际断言的 test-marker 程序，输入和产物均被忽略；通过/失败报告由程序及其退出码产生。
- G4：独立生成对象接入公开 takeover、启动前原 stop outbox、当前 build 绑定及离线停止/重启取证；历史 pin/request/Run/input 不改。补兼容/错证明/原请求丢回执、取消优先和持久化失败验证。
- G5：原执行页只读区域消费真实结果和稳定 Run/artifact/SHA256 引用；注册 `testing.management-local-codex` 与专属 Playwright 驱动。真实场景结果另列，不以这些实现声明替代验收。

定向验证：`%TEMP%/steerline-tm-focused-delivery-20261020/summary.json`，42 项 passed、complete=true、零失败/skip，14.058 秒。命令为 Plan 中 TestTM 合并选择，另含 `TestProductionSourcesUseOnlyOwnedTestkitSupport`；覆盖 Runner 字节/来源、collect 原请求与未知恢复/明确 stale、CAS/不可变、缺日志/冲突顺序/过期 Git、真实 fixture 程序退出码和生成接管。真实联验发现候选预留仍只接受 TaskContract 引用；已修复为仅接受固定 Plan 同 commit 下两个测试 JSON 和固定 inline 说明，新增 `TestTMReservationAdmitsOnlyBoundOptionalTestReferences` 验证任意路径、跨 commit、说明改写、重复及无绑定快照均拒绝。任务契约不新增字段，原普通 Task 引用校验保持。

Web：三个定向文件 26 项通过（11:01），`npm --prefix web run build` 通过；最终 Go 断言加强后 `TestTMGenerationCompatibleBuildObservesOriginalWithoutResubmit` 仍通过。

### 固定 L1 的已有失败

`%TEMP%/steerline-tm-l1-20261020/summary.json`：passed=false，215.3 秒。新增驱动对 feedback 的命名工具依赖登记遗漏已修复，专门守卫已复测通过。其余三处在 **HEAD 0752537e 的独立 git archive 源码副本** `%TEMP%/steerline-tm-baseline-0752537e/` 用同名定向命令复现，不属于本功能新增回归，也不将 L1 改记成功：

1. `TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding/readable_changes_survive_technical_event_noise`：断言期待中文阶段名称，主干 query 返回 `implement.report`。
2. `TestTM04ExecutionBoundary`（另连带 HostCheckpoint/TaskDelivery 守卫）：主干 `projectconnection -> processwindow` 不在 HTTP consumer 的依赖白名单。
3. `TestUnifiedStartupLifecycleSharedPIDIndependentInstances`：主干遇独立坏配置 `z-broken.json` 时以 unresolved project census 拒绝 Runner 启动，和该测试期待有效兄弟项目在线不一致。

复现命令：在上述独立源码副本执行 `go test ./internal/storage/sqlite ./internal/testkit/boundary ./internal/app -run '^TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding$|^TestTM04ExecutionBoundary$|^TestUnifiedStartupLifecycleSharedPIDIndependentInstances$' -count=1`。三处均失败，原源码与本工作树未为此放宽规则或修改无关功能。未运行 Full 或无关 L2/L4。


### 真实功能闭环及尚未结算现场

正式命令 `go run ./cmd/test-feedback -level=L3 -suite=testing -env=live '-scenario=testing.management-local-codex' -out <新目录>` 固定 Runner 为本节前述 bf3ec58 / `0.2.0-dev.3-r2` / SHA256 `21ab747f…ce697`，未改 Runner 源码或安装环境。场景使用全新 new-debug-release 包并冻结 live；真实 Codex 生成，随后正式批次/控制接口建立原开发候选。测试结果完全由模板的实际断言程序输出，失败输入在忽略目录，不提交故意破坏的代码。

`C:/SteerlineDebug/dev-20261020-111523-0752537e-dirty`（j 包）：

- 生成 Run `ac4a2b76702da9255693b964b92a8c53`，原生来源 `codex.agentMessage.final_answer`，state=saved；任务候选 `4b27b1c5986c42ef815d999b292f5436` 绑定开发 Run `71d386a024a728c0e32ce3e14734de5b`，同为原生 verified final，workflow=development/2。
- 被测 commit `4c2103cc08b7b401f5664e480c4d1330626b957c`；通过执行 `4bd5ed73f520475cb13222155e3dc8d5`、失败执行 `297241a0122645f0b32c1740be5d029b`，前后均 clean 且同 commit。公开 API 第一轮 passed=1/failed=0，第二轮 passed=0/failed=1、history=2；各自 summary 的 artifact ID 与 SHA256 均由原 Runner 收集/读取。
- `web/tests/test-management.spec.ts` 两轮均 1 passed（3.6s / 3.8s）；同包 evidence/test-management 下保留 `pass-api.json`、`fail-api.json`、两个 browser 日志、截图和 trace，以及程序/Run/生成源记录。截图已检查，计划测试行、失败原因、证据摘要与历史可见。
- 原 Run 到 publish 阶段后因原生沙箱中的 Git credential helper 失败而 blocked，没有把发布/任务交付当测试通过前置。本轮另用外部正式 `gh pr list` 核对本次精确 head/base，返回 `[]`；`git ls-remote` 仅有本包 base，原 Run head 不存在。没有创建 PR，未触及其他分支。
- 此次原 `steerline stop`/`verify-stop` 未通过：Runner 的 service stop 仅关闭观察 writer，并不撤销 Run 的持久业务授权；原开发 Run 尚未显式 stop，因此 native stop status 为空。原失败回执与 proof.json 保留，cleanup_complete=false；不把服务退出当资源结算。场景已补“持久原 stop 请求→公开原请求/Run 查证 stopped→整包 stop/verify-stop”，不改变产品的服务关闭/重启语义。最终复跑结果在下方追加。

i 包 `C:/SteerlineDebug/dev-20261020-110841-0752537e-dirty` 首次真实 API 通过后，驱动的 Web source 环境引用被 prepare 清理，浏览器未启动，随后同样留下未结算原 Run `3dd1c855f6ceabc807cbc4fa95b1e893`。已修正为 prepare 前固定驱动路径，并在启动前验证绝对路径和测试文件；不修改已冻结现场。

**Runner 同构建接管缺口**：j 包以自己的固定 EXE 执行公开 `service verify-takeover --candidate <同 EXE> --state-dir <原 namespace>`，`compatible=false`；已完成生成 Run `ac4a2b76702da9255693b964b92a8c53` 被标为 `unsupported`，诊断 `unsupported or out of range result acquisition configuration`，同现场开发 Run 则 compatible。完整封套保存在该包 `evidence/test-management/takeover-after-stop.json`。i 包同样复现，`cleanup-takeover.json` 保留原响应。恢复清理尝试在这个前置拒绝处停止，未发送 service start、stop 替代请求或新 Agent 输入。两包和其专属 base 暂保留；不能通过绕过 takeover、改私有记录或清空数据库来宣称恢复/清理成功。此项是 TM-A04 真实兼容接管的未满足前置，受控调用方通过不能替代它。

较早 a–h 包及对应 `%TEMP%/steerline-tm-live-20261020-<字母>/` 失败证据均保留：首次项目 ready 前调用、依赖目录并发使用、inspect selector/Task 批次格式/dispatch fixture 前置、一次空生成保护和实际候选引用校验缺口。后续修复分别验证，不改写原失败。除早期未 ready 包的项目实例证明不足、i/j 两包资源结算未核实外，已启动包记录了 stop/verify-stop；未推送/未启动的包不算真实资源验收。所有现场均未 reset 或覆盖原配置。


### 最终真实场景（k 包，通过）

正式场景输出 `%TEMP%/steerline-tm-live-20261020-k/summary.json`：passed=true、cleanup_complete=true、errors=null，非零命中 `TestManagementLocalCodex` 一项，执行 315.539 秒；完整生成包为 `C:/SteerlineDebug/dev-20261020-112514-0752537e-dirty`，本包 `evidence/test-management/proof.json` 的浏览器通过/失败、停止及清理均 true。

- 原生成请求 `026cd1d608db4aefaa2f1d469d283958`，Run `f6942435176c8f5d5430b15591e9ee8f`；正式 CLI state=saved，原生 verified source 为 `codex.agentMessage.final_answer`，保存 `SteerlineFlow/plans/task.tests.json` 只选择 fixture.marker。
- 正式任务控制建立候选 `36e9ec573bb14102a3cc2fe4ea394b87`、开发 Run `8402cc9208fbecdd4164d59e0fedc47d`；固定被测 commit `a2e7c1e4f2a3aea93d1dafbf42677ebb46456ee8`。通过报告 `dd1f149a87334ebc9216fffd1d9c47ae` 与失败报告 `5ab0d64007cf4a3c9f1f876fc47583a1` 均由真实程序产生，退出 0/1，前后 clean/commit 不变，失败来自独立忽略输入。
- 两个公开 API JSON 分别 1/0/0 与 0/1/0，history=2；两轮 Playwright 均通过，截图/trace、程序输出、原生来源、不可变 artifact/SHA256 引用均在本包 evidence/test-management。
- 清理先持久 `native-stop.json`，仅向原开发 Run 发送一次 stop，再通过原 request-id 核验 accepted/completed、原 Run/input 与 stop_status=stopped（`native-stopped.json`）。随后正式 `steerline stop` 和 `verify-stop` 成功；原宿主/项目身份及两个原 Run 的 input_digest 均匹配，resources_settled=true。不是仅凭 PID 消失判成功。
- Driver 按 owned base 的原 commit/lease 删除并补查远端，cleanup_complete=true。另正式 `gh pr list` 精确核对本包 head/base 返回 `[]`，`git ls-remote` 对本包 base/head 均为空；没有 PR 或本包远端分支残留。不会因此删除 i/j 的未结算专属 base。
- 最后仅修正场景通用 evidence 的默认 components 标签：本场景应为 local-codex / GitHub owned base，而非通用 fixture 的 controlled。k 原始 scenario.json 中旧默认标签保留，其 test_management 原生来源、精确 Runner pin 和原 API/浏览器证据是实际环境证明；标签修正后完成 Go 编译/定向验证，不篡改 k 原报告或为标签再次调用模型。

干净结算后再次执行**同一个 EXE** 的公开 verify-takeover，仍 `compatible=false`，仅原生成 Run 被标为 `unsupported or out of range result acquisition configuration`，开发 Run 为 compatible；证据 `evidence/test-management/takeover-after-stop.json`。所以这不是 j 未结算状态造成的假象。TM-A08 已通过，TM-A04 的真实兼容接管仍须 Runner 修复此前置；本仓调用方的受控严格绑定/取消优先测试已通过，不放宽为无证明接管。

最终限制：本批未部署/提交/创建 PR；固定 L1 的三处主干已有失败仍在上节登记。i/j 原开发资源及专属 base 保留待原公开证明恢复；首次未 ready 包保留其不完整实例证明。所有失败、清理未知与成功补测分别保存，未修改/重置受保护安装目录或 Runner 私有状态。


### PR #365 主干冲突处理（2026-10-20）

合入主干 `64bba737`，保留其 CLI `verify-runner-takeover`、Runner LLM 边界、runnerbridge 候选停止 owner 与纯 runnerbinding 校验；测试生成复用 runnercontract 的公开 takeover 类型/请求摘要和 runnerbinding 的构建/兼容判定，保留生成停止 outbox 与原身份校验。首轮编译因主干删除 runnerclient 类型别名失败，完成引用调整后复测。

定向证据 `%TEMP%/steerline-tm-conflict-fixed-20261020-124724/summary.json`：15 个包、177 项（含子用例）通过，complete=true、零失败/skip，53.992 秒。报告中的原命令覆盖全部 `TestTM`、接管契约/绑定、启动停止 outbox、部署重启绑定、CLI 预检和调用方/测试支持架构守卫。此前基线中的 `TestTM04ExecutionBoundary` 在本次主干上已通过；不据此改写历史失败或宣称完整 L1 通过。495 个 Markdown、3039 个本地导航链接检查零错误，差异空白检查通过。本轮未重跑真实模型、前端或 Full；历史真实场景及 Runner 前置阻断沿用上述固定版本证据。
