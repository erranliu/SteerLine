# 执行权限与 Git 环境检查接入验收

日期：2026-10-16。独立归属 SteerLine 接入 [Spec](../spec/environment-check.spec.md) / [Plan](../spec/environment-check.plan.md)。本轮已授权并实现 SteerLine caller/HTTP/Web，无 Task ID、不改 execution-map；真实 Runner 发布交接待前置。Runner 的独立前置验收位于其仓库 `docs/acceptance/environment-check.md`，使用 SREC 编号；本文件仅验证 caller/HTTP/Web，不覆盖 SA 或 Runner 原生通过状态。

## 自动项

| ID | 通过条件 | 所有者 / 场景 | 当前状态 |
| --- | --- | --- | --- |
| SLEC-A01 | 固定 Runner 身份、公开 schema/capability、模式和完整项目 Repository/Publication；同版 Controlled 样例与真实 EXE 契约一致，start/stop/两种 inspect 和无 Check 拒绝正确，检查失败不当传输失败；错误目标/回执拒绝，缺能力不阻断历史页 | SteerLine SLEC-CALLER、SLEC-HTTP、SLEC-INTEGRATION | 结果见本轮实施记录 |
| SLEC-A02 | 原 start/stop 请求先保存，重复/未知/重启不自动重发，迟到/本地提交失败只找回原绑定；保存 Runner 权威摘要而非重算，低 revision/同版冲突/缺字段不覆盖可信缓存；缓存不驱动执行或改 Task/候选/交付 | SteerLine SLEC-CALLER | 结果见本轮实施记录 |
| SLEC-A03 | 项目认证/revision/跨项目命令守卫有效；GET 零启动/检查/清理；离线缓存保留源时间及不可用诊断 | SteerLine SLEC-HTTP | 结果见本轮实施记录 |
| SLEC-A04 | 页面明确项目/环境/Agent/模式与操作范围，完全访问显式；区分 unsupported 与现场缺凭据，Runner 分项/未核实/清理/适用性和未检查构建资源如实展示；未知尚无绑定时仅查询，关闭页面不取消、重复点击不重派 | SteerLine SLEC-WEB | 结果见本轮实施记录 |

Runner 前置支持/未支持组合的事实由 SREC acceptance 提供；本仓不登记四个原生组合为自己的验收项。命令、环境、时点、通过条件和证据登记按 Plan 执行，计划测试名须实施登记且非零匹配。

## 人工项

| ID | 内容 | 窗口与阻塞范围 | 状态 |
| --- | --- | --- | --- |
| SLEC-M01 | 设置/项目入口、权限模式解释、Git/PR 操作预览、未核实/清理和历史范围可读 | 自动项后维护者窗口，可后置，不阻塞无关工作；产品自动失败不可据此豁免 | 待维护者窗口体验 |

## 本轮审查结论与文档处置

审查对象为拆分前 Spec SHA256 `f9c5212bb35b4898640affdcf3ce00a5fafdbbb709a3906eac42bb670185bb0f`、Plan SHA256 `f45aa0c042eb7912ba5ee964a116fedfb797eb6e98a24bcb7c6c997d8e3ee5b7`，均为本聊天未提交草稿；没有旧产品验收结果需迁移。以下均为设计问题，修订文档不等于对应实现或原生冲突已解决。

| 级别 / 原位置 | 问题与影响 | 本轮文档修订 / 仍需落实 |
| --- | --- | --- |
| P1 / 原 Spec §1/§3、Plan R1 | 已区分确定性探针与 Agent 实测，但未固定与 SA02 的接线约束；full_access 与 SA04/SA08 全局禁止 bypass 冲突，认证“不额外强制 App”未说明现有沙箱仅支持 App 时如何处理 | Runner Spec §2 逐项交接：不改普通执行的无模型预检；SA sandbox 专项保持不变，full_access 独立；认证仅用实际公开类型。SA 最终作用域和接口仍须实施前统一，不能宣称冲突已在代码解决 |
| P1 / 原 Spec §2/§8、Plan R2/原生矩阵 | SA07 当前直接拒绝远端，却要求四组合一并实测才交付，暗含扩展 Orca 原生协议且阻断可用本机 | Runner 按组合发布，远端缺能力继续 unsupported/待前置，不由 SteerLine 绕过；本机可先集成，远端目标仍保留且须单独真实验证 |
| P1 / 原 Spec §4/§5/§6 | 固定一次模型回合承载多次外部写，仅 start 幂等不能阻止回合内重复创建或未知后继续；base 分支由谁创建也未说明 | Runner 要求逐外部写动作原意图/目标/结算及同轮重复/丢回执反例，明确 Agent 创建 base/head；禁止仅用 prompt 保证，实际原生端口约束须在 Runner 实施落实 |
| P2 / 原 Spec §7/§8、Plan R1–R2 | Runner 细则由 SteerLine 定义且两仓共用验收；拆为独立前置后会出现双权威与重复维护 | 本轮拆出 Runner Spec/Plan/SREC acceptance，SteerLine 仅保留消费契约和 SLEC acceptance；实际集成固定发布版本 |
| P2 / 原 Spec §3/§7、Plan S2 | 首期即引入五命令、事件游标、独立 cleanup 和每次外部 canary；允许填写公开 execution 参数却未限制界面范围，容易扩成通用编辑器 | 收敛三命令、inspect 快照轮询、stop 统一收尾；只做必要字段表单；canary 留 Runner 原生定向验收，不重复做用户现场沙箱认证 |
| P2 / 原 Plan 测试表 | 只匹配新增 EnvironmentCheck 测试，可能漏掉普通业务 thread/turn/command 权限与认证回归 | Runner Plan 增加现有权限/认证定向回归；caller/HTTP/Web 与原生检查分仓验证，不要求无关 Full/整套开发 smoke |

## 文档验证与结果登记

上一轮仅整理跨仓草稿，产品项均未执行。本轮审查后将其拆分为两仓独立文档，原 EC 编号未有产品结果，直接采用 SLEC/SREC 各自编号，不建立兼容映射。

本轮只修改文档；未启动 Agent、服务、环境检查或 Git/PR 操作，未修改 `8876/SteerRunner` 沙箱认证工作树。静态检查通过：SteerLine 443 份 Markdown / 2651 个本地链接 / 0 错误；Runner 29 份 / 133 个 / 0 错误；两仓 `git diff --check` 及六份新规格/计划/验收的尾空白/冲突标记检查均通过。Runner 链接检查复用本仓检查脚本，仅切换扫描根，不向 Runner 新增检查工具。远端发布后链接未作上线可达验证。

后续实施结果追加实际源码/dirty、固定 Runner EXE、命令/非零用例数、状态/证据位置；原始日志放新临时目录，长期只留脱敏摘要。

## 拆分后第二轮审查（2026-10-16，仅文档）

输入为拆分后本仓 Spec SHA256 `eb5c1de3c68129275f72fd05365278cd03410a7fe03afca63f429b2b887a05db`、Plan `fe1cb6a6d3bb0424f6c7e74b9ee802d642bd4155b1d4628fed76a37bb7f6d988`，Runner Spec `39d40e9e4b51cc559bd643a089afeffd9aab4e75a31051020e7674116d530c1c`、Plan `42a1adb57777a3d65cefe159b9354602dc457bb937105002891c5c0b59925e95`；均未提交。核对 SA 在途 Spec/Plan/验收，不修改其工作树。

| 问题 | 修订与验收归属 |
| --- | --- |
| SA 准备可复用与其自身交付门槛未区分；full_access 的预检可能反向依赖 SA 实现全访问 | Runner §2.1 增加按时点/模式前置；真实发布/集成须 SA 相关最终提交合入并满足原交付门槛，当前有效凭据正例及所要求 L2 smoke 不豁免；全访问预检由本功能实现，SREC-A01 |
| capability 把缺实现机制与现场缺凭据混为 unsupported | Runner 固定机制支持、现场准备 failed/unverified 分开，SREC-A05；SteerLine 解析/展示分别纳入 SLEC-A01/A04 |
| 跨仓缺 stop 请求查询、拒绝无 Check、失败报告与 CLI 失败的返回约束，摘要/revision 更新口径未固定 | Runner §4.1 定义最小回执/Inspection/错误码；公开同版样例经 Controlled 核验后交接。SREC-A02、SLEC-A01/A02；未知无绑定不猜测 stop |
| 无 workflow 输入却用“项目流程要求 PR”判适用；项目 Publication/base 与检查 PR 目标容易混用 | 用明确 Publication 固定 PR 必需项，本仓不得漏传项目发布要求；测试 PR 仅检查 base/head，SREC-A03、SLEC-A01 |
| Runner Plan 允许 driver 准备 base，可能代建待验检查引用；SA canary 复用仅要求同原生版本过弱 | driver 仅准备 fixture source/base，检查 owned base/head 须 Agent 完成；证据复用还需策略/根目录/账户/目标未变，SREC-A01/A03/A06 |
| 实际 EXE 仅核对 capabilities/schema，却映射本仓持久请求验收 A02 | SLEC-INTEGRATION 改为仅 A01；A02 由 caller/SQLite 定向故障测试承担，不夸大静态契约覆盖 |
| Runner A01–A04 含真实权限/Agent/Git/收尾条款，但场景列只有 L1/Controlled | 增加 A06 对应组合的原生证据关联，Controlled 只证明规则/协议，不能单独签收真实能力；两仓验收与 SA 原任务继续分别负责 |

本轮只修正文档；两仓产品项继续未实施/待验。静态复验通过：SteerLine 443 份 Markdown / 2651 个本地链接 / 0 错误，Runner 29 份 / 133 个 / 0 错误；两仓 `git diff --check`、六份新文档尾空白/冲突标记检查均通过。没有执行产品或原生测试，没有修改在途 SA 工作树。SA 有效凭据正例/所要求 L2 smoke 与远端原生机制仍为实际前置，不能以文档互相引用标记已满足。


## SteerLine 实施与验证（2026-10-16）

源码基线 SteerLine `6f40f2dd70c2226c38e1599f81546652f475d563`，本工作树未提交；已包括本功能 Go/Web/文档变更。未修改 Runner 代码及 `8876/SteerRunner` 在途沙箱工作树；未运行真实 Agent、外部 Git/PR 或安装现场调试。

实现：独立 environmentcheck 用例、runnerstate 纯记录、SQLite schema 22 CAS/项目绑定与请求索引；公开 API 由应用投影为 JSON，分层守卫未放宽。HTTP 复用项目认证与唯一请求仲裁，start/stop 的写意图先保存，重复/未知/重启只查原请求；页面经执行设置进入，显式双模式与执行预览、分项证据/实际身份、源时间/适用性、资源待处理及原 stop 收尾均已接入。reset 保留未知原操作与未结算资源；不修改默认方案或冻结调试模板。

### 范围签收

| 自动项 | 本轮结果 | 尚待条件 |
| --- | --- | --- |
| SLEC-A01 | caller/CLI 子进程、schema/能力缺失/模式、完整项目 Publication、HTTP 独立路由与错误绑定通过 | **未整体签收**：Runner 正式同版 Controlled 样例、固定发布 EXE/version/SHA/resource_digest 和真实公开契约核对未交接；本仓 fixture 不能替代 |
| SLEC-A02 | caller/临时 SQLite 通过：并发同 ID 零重派，保存失败零派发，外部成功但绑定提交失败只查回原绑定，重启使用原 EXE pin，未知无 stop、停止原请求绑定、同 revision 不能再次收尾；权威摘要/同版冲突/回退/输入漂移保护、Task/候选零创建及 reset 恢复记录保护通过 | Runner 原生幂等/停止/资源证据另由 SREC 验收，本项只签调用方行为 |
| SLEC-A03 | 真实本地 HTTP handler、认证/项目/实例/revision 守卫、GET 零派发、unready 历史读取及离线缓存保护通过 | 真实 Runner 环境可用性结论不在本项签收范围 |
| SLEC-A04 | Web 自动通过：模式默认沙箱、完全访问显式授权、范围预览、重复点击保护、支持机制的现场凭据失败与 unsupported 区分、报告/资源/源时间/适用性、未知无绑定只查询、后台/卸载停止轮询且零取消 | SLEC-M01 人工体验后置待安排；真实 Native 结果显示待正式 Runner 交接 |

### 来源与证据

初轮公开拟议样例（后续修订见本轮复审）：[v1.json](../../internal/testkit/checkfixture/v1.json)，SHA256 `45ab5b50d5e6444781b3a01c257900d264eff6594b73d16c3c83f5c1eff1563b`。样例 source 明确为 SteerLine 提议，**非 Runner 已发布或 Controlled 核验样例**；其中 resource_digest、身份、命令/检查 ID 与失败报告均为测试数据，无真实权限含义。CLI 使用编译的 Go 测试 EXE，不能登记为真实 Runner 构建。

Windows/amd64、Go 1.26.4；临时 Git/SQLite 仅使用测试夹具，不访问安装目录。主要回归命令：

```powershell
go run ./cmd/test-feedback -- ./internal/environmentcheck ./internal/runnerclient ./internal/storage/sqlite ./internal/app ./internal/product/httpapi ./internal/product/applicationhttp ./internal/testkit/boundary ./tests/e2e/internal/driver -run '^(TestEnvironmentCheck|TestHTTPCommandBoundary|TestThreeLayerRepositoryBoundary|TestSteerRunnerCLIContract|TestUnifiedStartup|TestNativeDatabaseInitializesCurrentSchema|TestPreflightRejectsUnknownSQLiteSchemaWithoutMutation|TestDebugResetBeforeInitialization)' -count=1
npm --prefix web test -- src/features/environment-checks/environment-checks.test.tsx src/shared/api/queries.test.ts src/shared/api/client.test.ts
npm --prefix web run build
python scripts/check_doc_links.py
git diff --check
```

Go 8 包、56 项（含子测试）：55 通过、0 失败、1 跳过；日志 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261016T050111.439688200Z/summary.json` 及同目录 go-test.jsonl。唯一跳过 `TestEnvironmentCheckPublishedContract` 因未提供正式 `STEERRUNNER_EXE`，**不是实际集成通过**。正式执行该项还须指定 `STEERRUNNER_VERSION` / `STEERRUNNER_SHA256`，校验固定 EXE 及资源/schema。

Web 3 文件 / 38 项通过（含本功能 7 项、Query 定向失效和既有 journal/原请求约束）；TypeScript/Vite build 通过。原始 Web 测试/构建输出保存于 `C:/Users/lmas1/AppData/Local/Temp/steerline-environment-check-web-f6c0f8e1c64b4c07b415017647f15a70/tests.log` 和 `build.log`，源时间、后台/卸载、重复点击和离线缓存均使用测试夹具，无 Native 结果。构建产物为忽略目录，不作为已提交发布包。

最后对 SQLite 行版本核对、阻断旧未结算检查先于任何新 Runner 准备，以及拟议 fixture 的公开认证引用做定向复验；结果追加于下。未执行 Full/L2/L3：本轮没有改 TaskDelivery/业务 workflow 或 Runner 原生；共享入口、初始化/schema 和分层均已由上述定向回归覆盖。SA 有效凭据正例/L2 与远端能力前置保持原状态，不能据本轮调用方验证豁免。


定向复验通过：caller/CLI/SQLite/应用四包，日志 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261016T050604.093062800Z/summary.json`；原生契约测试仍因无固定 EXE 跳过。浏览器公开能力 `supported=null` 时历史可读且不能开始检查，已加入第 7 项 Web 用例；未知请求的 CLI 本地诊断也只指向查询原 ID。

最终补齐 Web 静态深链入口 `/projects/{project_id}/environment-checks`；`go run ./cmd/test-feedback -- ./web/server -run '^(TestEnvironmentCheck|TestWebAuthenticationAndOriginalForwarding|TestWebOriginHostAndStaticBoundaries)' -count=1` 三项通过，证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261016T050715.018176400Z/summary.json`。覆盖直接打开/刷新页面、三种只读 proxy 路由、原 start/stop 信封/版本保持及 CSRF，不启动实际 Runner。映射 SLEC-A03/A04。

最终文档静态检查：443 份 Markdown / 2657 个本地导航链接 / 0 错误；`git diff --check` 通过。实际 Runner/SREC/SA 与人工 M01 继续待前置，接入不记为完整原生产品验收完成。


## 实现复审与修复（2026-10-16）

范围：本聊天上轮未提交的 SteerLine 实现；只读取 Runner 草稿确认职责，未修改 Runner 仓库、在途沙箱工作树、原生权限/认证实现或外部业务状态。

| 级别 | 确认的问题 | 已落地修复与验证 |
| --- | --- | --- |
| P1 | 浏览器通用 reconcile 在 command_not_found 时自动 POST，环境检查意外继承了补交路径 | start_environment_check / stop_environment_check 在不存在、断连和未知时均只查询原 ID；重复 submit 原信封也只对账。client.test.ts 新增两动作 × 两种不存在码，断言零 POST，并保留旧 Task 的既有补交行为 |
| P1 | 检查专用 PrepareEnvironmentChecks 可重启常驻 Runner，绕过项目冻结的原实例，使既有 Admit/Shutdown/Settlement 仍指向旧实例 | 删除专用服务启动方法；新检查写请求复用 [原实例准入](../../internal/taskrunner/actor.go)，无 Task/DAG 资格操作。GET/原请求恢复不要求新输入准入；准入失败零 start/stop，既有 shutdown/settlement 继续负责同一实例 |
| P1 / P2 | 读取原 stop 先轮询 start 等全部历史命令，旧 start 查询失败会阻塞 stop 对账；每轮查询成本随历史操作数增长 | [caller 刷新](../../internal/environmentcheck/service.go)改为原命令只查该 request_id；详情优先查唯一未知操作，已绑定后只按 check_id 读快照。测试让旧 start 查询失败，确认 stop 可独立恢复且后续每轮仅一次 check 查询、零写派发 |
| P1 | 新检查重复读取 Git remote，绕过已有静态目标绑定与非秘密 URL 校验，可能把错仓库或内嵌凭据写进公开输入/历史 | 复用 [TaskRepositoryTarget](../../internal/app/task_repository_target.go)，只做现有静态项目目标装配，不调用 Task 资格或原生权限测试。应用测试确认 publication 错配及 URL 内嵌凭据均拒绝，错误与返回输入不携带伪造 token |
| P2 | 高 revision 报告可改变冻结必需项集合；JSON 相等比较经过 float64 会把相邻大整数视为相同不可变输入 | 拒绝必需项集合漂移，允许集合相同的排序变化；使用 UseNumber 保持整数精度。caller/SQLite 新增反例；仍直接保留 Runner 的 unverified/failed 等结论，不根据部分证据重算权限通过 |
| P2 | HTTP 仲裁文件丢失但领域原绑定仍在时，同信封重交被当成冲突；stop 的 null payload 被当成空对象 | 返回原绑定回执且零新提交；不同信封同 ID 继续冲突；stop 严格要求 `{}`。HTTP 定向用例覆盖 |
| P2 | 历史列表缓存 settled 时，选中详情最新 pending 未能阻断再次检查按钮 | 选中检查以最新详情覆盖历史摘要计算资源状态；Web 用例确认按钮保持禁用。后端持久状态校验继续是最终写入保护 |

职责结论：保留独立的小型 caller 请求/绑定/缓存用例及既有 SQLite/HTTP/browser journal；没有新增通用 outbox、调度器、worker 状态机、事件流或自动审批。原生执行、认证、沙箱/全访问映射、检查取证、分项结论与资源收尾仍由 Runner 单写。已移除本仓额外常驻服务启动与历史命令全量轮询；公开 DTO 的 JSON 投影用于现有纯依赖边界，不放宽架构守卫。

本轮拟议 fixture 将按 check_id 的 command:null 纳入 schema，与 Runner 草稿的可空 command 约定一致。当前 [v1.json](../../internal/testkit/checkfixture/v1.json) SHA256 `aedfcaf6c2899e4a9feabb7dfb43cb881193572bce7175b38a6d9ab9b64c9092`；初轮记录的旧 hash 只代表当时样例。本轮仍非 Runner 正式发布/Controlled 样例，SLEC-A01 的真实契约核对、SREC/SA 原生前置及 M01 不据此签收。

### 本轮测试计划与结果登记

本轮 Plan 继续映射 SLEC-A01/A02/A03/A04：定向 caller/CLI/SQLite/应用/任务协调器、HTTP 与依赖边界；浏览器原请求、Query、feature；TypeScript/Vite build 与文档静态。使用 Windows 临时 SQLite/Git/公开 CLI 测试夹具；没有运行真实 Agent、外部 Git/PR 或 Full/L2/L3。新增的服务准入共享项由 TestEnvironmentCheckSharesOriginalRunnerAdmission 与 TestEnvironmentCheckAdmissionFailureDoesNotStartOrReplaceRunner 覆盖；既有 TestRunnerShutdown*/TestRunnerSettlement* 验证原停服与资源结算无回归。

```powershell
go run ./cmd/test-feedback -- ./internal/environmentcheck ./internal/runnerclient ./internal/storage/sqlite ./internal/app ./internal/taskrunner ./internal/product/httpapi ./internal/product/applicationhttp ./internal/testkit/boundary ./web/server -run '^(TestEnvironmentCheck|TestHTTPCommandBoundary|TestThreeLayerRepositoryBoundary|TestTaskRepositoryTarget|TestRunnerTaskServiceFreezesOriginalStart|TestRunnerShutdown|TestRunnerSettlement|TestUnifiedStartup|TestWebAuthenticationAndOriginalForwarding)' -count=1
npm --prefix web test -- src/shared/api/client.test.ts src/shared/api/queries.test.ts src/features/environment-checks/environment-checks.test.tsx
npm --prefix web run build
python scripts/check_doc_links.py
git diff --check
```

Web 3 文件 / 43 项通过，TypeScript/Vite build 通过；原始日志 `C:/Users/lmas1/AppData/Local/Temp/steerline-environment-check-review-ab2c48e4a27d40cea05b4a72a8c774fa/web-tests.log`、`web-build.log`。Go 最终汇总与静态结果登记于下。

Go 最终回归：9 包 / 66 项（含子测试），65 通过、0 失败、1 跳过；证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261016T052657.880224800Z/summary.json` 及同目录原始 go-test.jsonl。唯一跳过仍为无正式 EXE 的 TestEnvironmentCheckPublishedContract，不将其计为 Runner 集成通过。源 commit `6f40f2dd70c2226c38e1599f81546652f475d563`，当前工作树未提交。

本轮静态复验：443 份 Markdown / 2661 个本地导航链接 / 0 错误；git diff --check 通过。Runner 源码及其 Spec/Plan 均未修改。

### PR 同步主干后复验

提交 `f89d5e1001d78d18ff96aca020f3c7701b2d6e13` 基于主干 `3dcbdc41`，测试时工作树干净。同步保留原生会话查看与环境检查两项 capabilities；沿用上列 Go 命令并在 `-run` 中加入 `TestNativeSession`，9 包 / 70 项（含子测试），69 通过、0 失败、1 跳过。证据 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261016T065651.768781800Z/summary.json` 及同目录 `go-test.jsonl`。唯一跳过仍是缺少正式 Runner EXE 的 TestEnvironmentCheckPublishedContract。

Web 沿用上列测试命令并加入 `src/features/runs/native-session.test.ts`，4 文件 / 45 项通过；TypeScript/Vite build 通过。文档检查 446 份 Markdown / 2666 个本地链接 / 0 错误，diff --check 通过。本轮 Web 与静态证据见当前 Codex 聊天命令输出；未运行真实 Runner 联调或改变待验范围。
