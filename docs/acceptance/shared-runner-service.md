# 应用级共享 SteerRunner 服务验收

归属：2026-10-19 普通开发，[Spec](../spec/shared-runner-service.spec.md) / [Plan](../spec/shared-runner-service.plan.md)。只覆盖 SteerLine 共享服务接线，不覆盖 Runner 原生执行/恢复专项或闪窗治理。

状态：已按用户授权实施，SRS-A01–SRS-A07 自动验收通过；SRS-M01 人工体验待验。未使用或修改安装环境，未修改 Runner 仓库，未执行真实模型/业务 Run 或创建 PR。

## 文档审计（2026-10-19）

本轮对照现有 owner、QueueControl/outbox、私有停机 DTO、verify-stop、debug driver 和测试入口审计，以下问题先修正文档，再随本次代码实施。

| 级别 | 原问题 / 代码依据 | 已确认的修订 |
| --- | --- | --- |
| P1 | `runnerbridge.QueueControl` 只按 Runner RequestID 去重；单纯改为随机 ID，会让“Action 已存、外层回执未存”的重试绕过去重 | Action 保存产品请求 ID，在原候选 gate 内先查旧动作/正文再生成外部随机 ID；加入该崩溃窗口与并发反例，沿已有 outbox 保存 |
| P1 | 原 `bindRunnerBuild` 在项目阶段、service start 前校验；改为先启动共享服务会让错构建先打开旧状态，reset 保留历史后不能靠项目记录兜底 | 将服务构建准入上移为应用私有不可变 BuildPin，先核验再启动；缺 pin 但目录已存在即拒绝，失败/reset 保留 pin；项目记录仅保留原操作快照，测试断言拒绝路径零 service start |
| P2 | 项目触发共享初始化增加首次申请、等待者取消和所有权交叉，当前 `ApplicationDirectory.run` 已有 census→项目启动顺序 | 应用在 census 后直接初始化一次，再注入项目；允许后续 prepare 全失败时保留空闲服务，不为省空闲进程加两阶段预备 |
| P2 | scope SHA-256 加随机 operation 已由随机部分保证唯一，却要求 home/project/动作注入和专用 ID 协议 | 外部请求只用独立随机 ID；产品原请求与信封持久关联，归属按项目库/Run/Check/correlation 核验，不给 contract 层增加生成器 |
| P2 | 一份共享 ResourcesSettled 复制到所有 ProjectShutdown，停服后再回填；离线证明也按项目重复嵌入 | 项目只记本地关闭/锁；应用保留一份 Runner 结算，verify-stop 也只保存一份共享证明，不维护第二份项目资源状态 |
| P2 | 项目 actor 关闭后仍笼统承诺原停止义务继续处理，实际已无该 actor 对账者 | 义务持久保留；由整体停服结算资源，或显式重启后原流程核验，不新增后台项目结算器 |
| P2 | 新 PowerShell wrapper、独立报告约定、三个预设 L1 组、重复包全量/L3/L4/Edge 把拓扑调整扩成测试基础设施工程 | 保留必要 L1、一个新双项目 L3、既有等待合并重启及 debug driver 真实空闲服务测试；复用入口/产物，不默认 L4/Edge 或新脚本 |

保留的必要改动：共享 owner/目录、原请求绑定、全局停服/离线证明、环境检查 Checks 消费、整包 reset 与 fixture provider 路由。它们均有现有消费者，不能靠仅改 state-dir 完成。新目录不迁移旧状态；reset 保留整个 Runner 目录，不为清除历史增设 Runner API。

## 实现审计（2026-10-19，提 PR 前）

对照 Spec 3–6 与 Plan A–D 逐项审计，确认并修复以下问题；本轮没有新增服务池、租借计数、后台结算器、恢复 journal 或 Runner API。

| 级别 | 已确认问题 / 触发 | 修复与最小反例 |
| --- | --- | --- |
| P1 | 项目重建只核对候选 Run，未按 C6 核对已存 Check/未知请求，可能在旧检查绑定未核实前开放项目 | `VerifyBindings` 在项目 actor/端口创建前复用原检查 schema 与身份校验；无写回/新输入。`TestSharedRunnerRestartVerifiesChecksReadOnly` 覆盖正常、未知已受理/不存在、错 Check/input/command、离线/错目录；`TestSharedRunnerCheckBindingFailureIsProjectLocal` 核对本项目失败、兄弟 ready、共享实例未停未换 |
| P1 | 继承旧项目 owner 的 `before.Status` 分支：已有可信共享实例启动成功后，目录查询失败会丢失 stop 闭包，整体关闭无法结算 | 服务身份核实后即固定原停止义务；删除前置 status 查询及新/旧服务分支。`TestSharedRunnerCatalogFailureStillClosesAdoptedInstance` 修复前复现 `original Runner shutdown remains unverified`，修复后沿同一原实例完成结算；身份未核实的启动仍不得猜测停止目标 |
| P2 | reset 仍要求缓存 Check.cleanup=completed；项目库先关闭、Runner 后停止时，缓存可能永远停在 pending，覆盖已取得的全局最终证明 | 仅以本次共享证明匹配 Check ID/input_digest/settled，保留未知操作阻断；同一 helper 核对目录/原资源，不新增结算状态。`TestEnvironmentCheckDebugResetPreservesUncertainResources` 覆盖旧 pending+新证明通过、缺/错证明拒绝、未知操作保留 |
| P2 | 检查新增 state-dir 未纳入 SQLite 既有不可变比较；stop 路径也缺少原目录检查 | 原 CAS 冻结 state-dir；提交 stop 前核对记录项目/目录。`TestEnvironmentCheckStoreCASAndIsolation` 与 `TestSharedRunnerCheckStopRejectsOtherDirectory` |
| P2 | Action 新增的原产品身份/信封只在用例去重，存储更新仍可覆盖或移除它们 | 在既有 SaveCandidate CAS 中核对原 Action 前缀的产品 ID/摘要、命令及完整信封，观察状态仍可更新。`TestRunnerCandidateReservationAndProjection` 增加移除/替换身份、命令及信封反例 |

结构结论：应用拥有一个 Runner，项目保持自己的 actor/库/锁与固定客户端；正常关闭、未知停止补查、最终资源结算沿同一 owner，不增加并行状态机。项目退出后已受理执行可能继续，是 Spec 明确选择的行为；不以本地关闭冒充资源结算。整机共享、项目热重启、旧布局迁移及闪窗治理继续不在本次范围。

本轮反例初始化曾误用只读项目 store，已改为正式初始化后打开命令 store；该测试准备错误与修复前的真实失败分别保留在聊天工具输出，未操作安装环境。定向初轮日志 `%TEMP%/steerline-shared-audit-directed.log` 保留失败，修正后的单场景通过后纳入固定 L1；最终验证证据见下文后续登记。

## 自动验收

环境、命令、时点、通过条件及证据一一映射见 [Plan 测试计划](../spec/shared-runner-service.plan.md#3-测试计划)。

| ID | 必须核对的事实 | 级别 / 入口安排 | 状态 |
| --- | --- | --- | --- |
| SRS-A01 | census 后应用初始化一次，两项目同绑定；零项目/无可启动项目/Close-before-Start 零启动，后续 prepare 失败不重启/误停，初始化与整体停止竞争，跨 home 隔离 | L1 owner/path；新 L3 双项目；真实包补拓扑 | 通过（见证据登记） |
| SRS-A02 | home 短路径/socket/别名拒绝，全部消费者同源；不可变应用 BuildPin 在 service start 前核验，错构建/缺 pin 旧目录零启动，失败/reset 保留 pin；候选/检查原绑定不迁移 | L1 path/owner、引用审计；真实包 | 通过（见证据登记） |
| SRS-A03 | 跨项目/业务/检查同产品 ID 独立；Action 已存而外层回执丢失、Run revision 前进、并发重试都复用原产品请求关联的信封；改产品正文拒绝，unknown/重启只读，错 Run/Check/digest/correlation 拒绝 | L1 taskrunner/bridge/environmentcheck；新 L3 同号请求 | 通过（见证据登记） |
| SRS-A04 | 项目仅关本地、义务保留且不伪称仍在对账；整体门槛/排空/原停止/最终证明/预算及失败保锁，只有一份全局资源结论；实例换代围栏不变 | L1 shutdown/binding；新 L3 正常整体停启及既有 restart；真实包 | 通过（见证据登记） |
| SRS-A05 | verify-stop 一份共享 Runs/Checks 证明覆盖全体绑定，离线未知请求阻断；reset 全体预检、无库不绕过，保留应用构建 pin/共享历史/认证/Git/PR，新操作不误用旧回执；部署/cleanup 不重复停服 | L1 client/app/driver；真实包 reset，原失败回执和库 hash 核对 | 通过（见证据登记） |
| SRS-A06 | 两 Run 同时存在、同产品请求 ID 不串、任务/事件/PR独立、整体停启零自动输入，原等待合并候选可继续只读核对，清理完整 | 新 `runtime.shared-runner-projects` / `TestSharedRunnerProjects` 与既有 `recovery.waiting-merge-restart`；实现时同步 suites.json/清单/选择校验 | 通过（见证据登记） |
| SRS-A07 | 新包两个项目实际 ready、snapshot 同 path/build、真实 Runner 身份唯一；重复 start/正常整体停启/reset，目录保留及清理可核实 | 既有 debug driver 内新增 `TestDebugReleaseSharedRunnerService`，精确选择；不新增 wrapper/报告协议/生产测试字段 | 通过（见证据登记） |

最小范围不含 Full、完整 L2、L4、Edge 或 Runner 原生故障矩阵。未改 Web/HTTP 业务与写者准入时，不重复已有系统边界和浏览器主流程；实际实现扩大这些边界时再选择既有场景并登记原因。A06 为公开 CLI fixture 双 Run，A07 为真实空闲服务，两者都不冒充真实模型双 Run。

## 执行顺序与现场

1. 固定实际 SteerLine commit/dirty、Runner 来源/version/SHA/resource digest；完成 Plan 的 owner、请求、fixture/driver/场景能力后运行定向及固定 L1，检查非零匹配/终态与受影响消费者编译。
2. 按 Plan 精确运行新 L3 与 waiting-merge-restart；各自拥有 home/repo/provider/证据，沿原并行和清理约束，不消费其他场景产物。
3. 使用现有 `scripts/new-debug-release.ps1 -WebProjects 2 -SteerRunnerExe <固定绝对EXE>` 在 `C:/SteerlineDebug` 新建真实包，将路径传给 `STEERLINE_DEBUG_SECOND_TEST_ROOT`，只运行新增 `TestDebugReleaseSharedRunnerService`。沿默认 live 冻结，不提交任务/模型/PR；缺固定 EXE/身份或意外 skip 则该项未通过，不降级 simulate。测试结束恢复环境变量。
4. 逐项核对原请求/停机证明、进程/租约/资源停止及 reset 保留事实，失败和补测分别登记，原始诊断不覆盖。无需为本任务默认另建 simulate 浏览器包。

分级 E2E 使用自身隔离夹具。不得使用、修改或重置 `C:/SteerlineInstall`、`C:/SteerlineRuntime`，不复制其数据库/配置/认证。若另行开展真实 PR 调试，仅限 `erranliu/SteerlineTest` 本次专属 base/head，并另行明确验证范围。

reset 与销毁测试夹具区分：reset 只重置 caller 状态，保留共享 Runner 目录；销毁只在全体资源停止可核实时处理本次拥有的临时现场。未知操作或未结算写者不能靠清目录转为通过。

## 证据登记

沿现有 test-feedback 的 summary.json / `scenarios/<id>/scenario.json` 和 debug 包 evidence 保存原始产物；仓库只记录必要摘要/定位。不新设报告协议。每次登记实际命令/选择、source commit/dirty、构建 SHA、Runner 身份、结果、证据路径和清理；真实包另存公开 status、项目 snapshot、原停机结果及 reset 保留事实。产品验证与上文文档审计分别计。

| 日期 / 版本 | 验收项 / 实际选择 | 结果 | 证据与清理 |
| --- | --- | --- | --- |
| 2026-10-19 / `abd001f1` + 当前 dirty | A01–A05：`go run ./cmd/test-feedback -level=L1`，含既有 owner/path/outbox/Checks 及 Task/架构边界 | 通过；两轮固定 L1 分别 114.6s、137.8s，保留首轮和补测 | `%TEMP%/steerline-test-feedback/20261019T015228.751005400Z/summary.json`、`20261019T020151.262789000Z/summary.json`；均 `complete=true, passed=true`，无安装现场 |
| 同上 | A01–A05：app / runnerclient / bridge / driver 的 owner、停止取证与 reset 定向测试（精确命令见下文） | 通过；四包，app 69.238s；失效停止结果/DB/原证明不改写，Checks 缺失、重复、未结算、错 home/input 反例拒绝 | `%TEMP%/steerline-shared-final-directed.log`；隔离 home、真实 SQLite 及公开 CLI peer，无原生执行资源 |
| 同上 | A03：taskrunner / bridge / environmentcheck 的请求定向测试（精确命令见下文） | 通过；产品回执丢失/已存 Action/版本前进、并发重试保持原信封，跨项目同号检查独立 | `%TEMP%/steerline-shared-control-final.log`；此前三包完整定向回归也通过，见 `%TEMP%/steerline-shared-runner-requests.log` |
| 同上 | A01、A03：`go test ./internal/app ./internal/environmentcheck -run '^TestSharedRunner' -count=1 -timeout=90s` | 通过；app 12.837s、environmentcheck 1.591s；新增初始化已受理但响应丢失时整体 Close 的竞争反例 | `%TEMP%/steerline-shared-late-start.log`；应用等待原初始化，保留 build pin/未知结算，零项目迟到开放；公开 fixture 明确停止原实例后销毁隔离目录 |
| 同上 | A05：`./scripts/test-deployment-stop.ps1`；登记/消费者：`go test ./cmd/test-feedback -count=1`、`go test -tags=e2e ./tests/e2e/... -run '^$'` | 通过；部署脚本只接受当前全局 Runner Runs/Checks 证明；清单校验 15.544s、八个 E2E 包编译通过 | `%TEMP%/steerline-shared-final-config-tests.log` 与本次工具输出；仅内存部署夹具，无安装读写；编译不计场景通过 |
| 同上 | A06：Plan 的两项精确 L3 / controlled，Windows 独立进程 | 通过；shared-runner-projects 154.81s、waiting-merge-restart 98.59s | `%TEMP%/steerline-test-feedback/20261019T015356.401656800Z/summary.json` 与 `scenarios/<id>/scenario.json`；两项 `passed=true, cleanup_complete=true`；保留双项目 before/after 快照、各 provider/Run/请求证据 |
| 同上 | A02、A05、A07：新真实双项目包，`TestDebugReleaseSharedRunnerService` 精确选择，Windows WMI 独立上下文 | 通过，87.89s；重复 start 同 PID/实例，整包 reset 保留 pin/共享目录，重启新实例，零业务 Run | `C:/SteerlineDebug/dev-20261019-100116-abd001f1-dirty/evidence/shared-runner-service.json`、日志、最终 status/snapshot/proof；最后 backend/Web exited、clean=true，Runner 所有资源已结算，包与短目录保留供查看 |

A06 补充 PR/provider 归属断言后，仅重跑 `runtime.shared-runner-projects`：`./scripts/test-runtime-isolation.ps1 -Level L3 -Suite runtime -Scenario runtime.shared-runner-projects` 通过（179.23s），报告 `%TEMP%/steerline-test-feedback/20261019T020630.458745000Z/summary.json` 与该场景 `scenario.json` 均为通过/完整清理；两项目 PR head branch 不同，各 provider 只保存自身外部 submit 回执。waiting-merge-restart 沿前一轮通过，不重复运行。

最终选择器校验再次通过（17.623s，`%TEMP%/steerline-shared-selector-final.log`）。`git diff --check` 通过，新增及修改 Go 文件格式检查通过。

定向命令（表内两组）：

```powershell
go test ./internal/app ./internal/runnerclient ./internal/runnerbridge ./tests/e2e/internal/driver -run '^(TestSharedRunner|TestDeployment|TestDebugResetBeforeInitialization|TestEnvironmentCheckDebugReset|TestSteerRunnerOutbox|TestUnifiedStartupLifecycleSharedPID|TestUnifiedStartupZeroProjects|TestUnifiedStartupCloseBefore)' -count=1 -timeout=3m
go test ./internal/taskrunner ./internal/runnerbridge ./internal/environmentcheck -run '^(TestSharedRunner|TestRunnerExplicitInput|TestSteerRunnerOutbox)' -count=1 -timeout=90s
```

owner 测试命名同步后，`go test ./internal/app -run '^TestSharedRunner' -count=1 -timeout=90s` 再通过（19.192s，`%TEMP%/steerline-shared-owner-final.log`），包括项目配置拒绝不停止共享服务、初始化响应未知、pin 准入与离线 Checks 绑定。

Windows 卷标大小写补测：`go test ./internal/runnerstatepath -count=1` 通过（1.497s）；同 home 的大小写输入输出相同目录字符串，真实 socket/重定向拒绝/清理断言通过，沿隔离路径销毁。

真实固定构建：Runner 来源 `59c7597ac8056fc5c29ad622034e10e74e4395de`，version `0.1.0-dev.1`，SHA256 `a989b95c9dfa68231fc1153044913542987446cac580d7cae255401a33ce7cc5`，resource digest `c689c62a5a86fd3217d4a635703149e6a6b5fd00c57a1edc26ea05f6c3b3ed99`。来源 EXE 为 `C:/Project/SteerRunner/bin/steerrunner.exe`，生成器复制至包内固定。生成 manifest 记录本次 SteerLine/backend/Web/helper SHA；源码为 `abd001f1fc40fb57abec49f4173874c0c942f2f1` 加当时未提交变更；测试时尚未提交，审计后统一提交并提 PR。

真实 Runner 目录 `C:/slr/ogubpylzeoh5x7ens6oii23mum`，两次 ready 实例分别 `3db51888f30e0b95eaa70cf13590d431`（PID 27080）与 `f0715612f2fc5b524439f566f5a48ab3`（PID 19788）；两个项目 checkpoint 都绑定同一目录/build，原应用停机结果包含一份共享结算、两个项目本地关闭/锁结果。原结果在包内 `home/application-api/shutdown.json`，最终公开证明 `evidence/shared-runner-final-proof.json` 为 `runs=[], checks=[], resources_settled=true`；`shared-runner-final-status.json` 与 `shared-runner-final-snapshot.json` 为停止后的只读补充证据。双 Run 业务接线由 A06 证明，真实包不调用模型。

失败与修正分别保留：

- 初轮 Go 定向测试暴露旧测试中的共享指针浅复制、缺少共享服务 fixture 和把产品 ID 当 Runner ID 的假设，已改为当前所有权/原信封断言后补测通过；测试选择器也同步 L3 runtime 从两项变三项。早期输出保留在当前聊天工具记录。
- `dev-20261019-095311-abd001f1-dirty`：直接 Go 测试在 kill-on-close Job 内，产品拒绝常驻资源启动；原 `evidence/shared-runner-service.log` 与 `%TEMP%/steerline-shared-debug-test.log` 保留。没有启动 Runner/业务输入；后续独立上下文验证。
- `dev-20261019-095810-abd001f1-dirty`：两个项目和真实 Runner 已 ready，但测试在启动器种子任务完成前 stop，导致 launcher 返回失败；应用与 Runner 已正常清理，包内 shutdown 为 clean。改用现有 `debugRunReady` token 等待完整准备，不新增 readiness 协议。原包日志与 `%TEMP%/steerline-shared-debug-independent.log` 保留；成功包是重新生成的 `100116`，不在失败包内替换方案或 EXE。

真实包默认 live 仅完成既有本包种子准备和空闲服务验证，准备分支限定 `erranliu/SteerlineTest` 的本包 `codex/debug/<release>/base` 与 `/base-b`。未启动候选、模型或 PR；Git/认证/分支事实按 debug/reset 原规则保留。未运行 Full、完整 L2、L4、Edge 或原生专项；HTTP/Web 业务和写者准入未改变。

## 实现审计后的验证

- 新增启动检查/错误收尾反例：`go test ./internal/app -run '^TestSharedRunnerCheckBindingFailureIsProjectLocal$' -count=1 -timeout=90s` 通过（32.882s）；同 home 一个检查错绑项目失败，兄弟保持 ready，最终共享停止成功。环境检查与 driver 的首轮定向测试分别通过（5.925s / 2.273s），初轮 app 的只读准备错误已单独修正。
- 冻结存储：`go test ./internal/storage/sqlite -run '^(TestRunnerCandidateReservationAndProjection|TestRunnerStop|TestEnvironmentCheckStore)' -count=1 -timeout=90s` 通过（3.337s），证据 `%TEMP%/steerline-shared-audit-storage.log`。
- 登记、消费者、部署：`go test ./cmd/test-feedback -count=1` 通过（21.024s，`%TEMP%/steerline-shared-audit-support.log`）；八个 E2E 包带标签编译和 `scripts/test-deployment-stop.ps1` 通过，不以编译替代场景验收。
- Plan 两项 L3 精确回归通过：`./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -Scenario 'runtime.shared-runner-projects,recovery.waiting-merge-restart'`；报告 `%TEMP%/steerline-test-feedback/20261019T022034.137612200Z/summary.json` 与两份 scenario.json 均为通过/完整清理。
- 审计后重新生成双项目包 `C:/SteerlineDebug/dev-20261019-102008-abd001f1-dirty`，沿既有 Windows 独立进程上下文仅执行 `TestDebugReleaseSharedRunnerService` 通过（139.29s，非 skip）。固定 Runner 来源/version/SHA/resource digest 与上轮一致，当前产品 EXE 的身份见该包 manifest。`evidence/shared-runner-service.json` / 日志、`shared-runner-audit-final-status.json` 和 `shared-runner-audit-final-proof.json` 取证；最终 backend/Web exited、clean=true，`C:/slr/nlqwfykrvpmf6arfxouocdejce` 保留，Runs/Checks 均为空且全体资源已结算。无模型/业务 Run、无安装环境操作。
- 固定 L1 首轮与 L3/新包构建同时执行，`quick-owner-unified-startup` 触及原有三分钟组预算；当时 44 个完成测试通过、无断言失败，其余未完，不能计通过。失败报告 `%TEMP%/steerline-test-feedback/20261019T021957.597929200Z/summary.json`、`%TEMP%/steerline-shared-audit-l1.log` 保留；保持原预算与测试选择，随后独立重跑，结果另记。

固定 L1 独立补测完整通过：`go run ./cmd/test-feedback -level=L1`，原组预算/选择未变，总执行 182.6s，报告 `%TEMP%/steerline-test-feedback/20261019T022542.833547300Z/summary.json` 为 `passed=true, complete=true`，包含新增全部 owner/检查反例及存储冻结校验。失败轮与补测轮分别保留，没有扩大为 Full。审计问题已修复，无未处理的已确认问题；SRS-M01 继续后置，不将人工体验签收写成自动通过。

## 人工验收

SRS-M01：自动项完成后，用户在本次新包体验双项目启动、失败项目“已有执行可能继续”诊断及整体停止。窗口待定，可后置，仅阻塞日常体验签收；闪窗治理不在本项。

状态：未安排 / 未验。
