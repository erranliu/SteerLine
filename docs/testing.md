# 测试总入口

可靠性交付简化的本批定向验证见[实施 Plan](plans/reliability-simplification.plan.md)和[验收](acceptance/reliability-simplification.md)：Go 部署目标/一次恢复、实际命令完整绑定、同快照观察与 CAS；新用例进入既有 L1 owner 分区，公开 CLI 联验另用实际固定 Runner，未运行完整 L2/L3 或安装环境。

部署中断自动恢复的最小验证见[Plan](plans/deployment-interrupted-recovery.plan.md)与[验收](acceptance/deployment-interrupted-recovery.md)：固定 L1 `quick-owner-deployment-recovery` 验证缺失/陈旧回执的恢复准入、readiness、新停机证明及失败围栏；CLI 参数归 `quick-owner-runner-preflight-cli`。脚本路由另运行 `scripts/test-deployment-stop.ps1`，不使用安装现场或真实 OS 重启。

近期合入 PR #352–#365 的补验和人工操作见[本轮导航](acceptance/merged-pr-regression-20261020.md)。固定 L1 将累积的应用启动测试拆为 `quick-owner-runner-startup`（接管、启动撤销与共享 Runner）和 `quick-owner-unified-startup`（项目/宿主生命周期），保留全部原测试及每组原三分钟预算；新增无法解析项目 census 时零共享服务启动的反例。覆盖检查按固定分区核对包终态，并拒绝跨 owner 重复执行，不用重复一组补缺失组。

调用方边界收口（2026-10-20）的最小验证见[Plan](plans/architecture-boundary-consolidation.plan.md)和[专属验收](acceptance/architecture-boundary-consolidation.md)：新 owner 依赖闭包、启动/正常 stop 共用 outbox、takeover 公开校验与原事实匹配、暂存部署 CLI 在线预检；只跑隔离定向测试，不使用安装现场或完整原生流程。

Runner 兼容构建接管的定向测试与未验边界见[专属验收](acceptance/service-generation-recovery.md)：公开 takeover/startup stops、当前 pin 与历史 candidate/Check 分离、启动撤销 outbox、兼容部署证据；不运行安装环境或以 fixture 代替原生资格。

Windows 启动闪窗修复的最小验证见[专属验收](acceptance/startup-console-windows.md)：固定 L1 `quick-core` 的 `TestBackgroundCommandsNoConsole` 从 detached 宿主核验四类后台命令的实际控制台句柄；另定向回归统一启动与共享 Runner，不运行全量或使用安装环境。

## 测试管理文档与结果

[项目维护](product-design/project-maintenance.md)统一初始化/基线/局部规则，生成先审阅、再显式应用，首次启动固定远端最新代码和同版本材料。必要材料待办阻断相关首次启动，测试通过不作为发布/交付门禁。实现及验收入口见 [Plan](plans/project-maintenance.plan.md#测试计划)和[专属验收](acceptance/project-maintenance.md)。L1 新规则仅登记 quick-owner-project-maintenance，app/SQLite/生成相关测试仍归原 owner；新增 L3 runtime.project-maintenance 使用唯一生成器、受控双项目 Go/Web/Edge 流程。

可映射到执行页的清单为 [test-list.json](testing/test-list.json)，格式遵循[测试管理 Spec](spec/test-management.spec.md)。每项必需 `id/name/level/environment/command/check`，仅 `note` 可选；拒绝未知字段、重复键/ID、空必需值与非法级别。首批登记固定 L1 整体及现存十三项 E2E 场景。E2E 执行选择仍以 `tests/e2e/suites.json` 为准；新增、删除、改级/环境/入口/主要断言时，依据通用规范同步修改实现、执行登记、展示清单和受影响 Plan。Web/脚本入口暂未登记，不推断它们已被覆盖。

`<name>.plan.md` 旁的 `<name>.tests.json` 只保存 `{"tests":["l1.minimum"]}` 这样的有序唯一 ID；Plan 链接它并解释选择。无需自动测试时可手工写空数组并说明原因。候选从固定 PlanRef 的同一 commit 读取安排/定义，缺失、无效或未核对的实际材料阻断相关首次启动；后续候选保留 Task 原快照。

应用已启动时，使用原生 CLI 生成已有 Plan 的测试安排；显式指定本机 Codex profile 和 32 位小写十六进制请求号：

```powershell
steerline tests generate -project <id> -plan docs/spec/example.plan.md -execution-profile <profile> -request-id <id>
steerline tests generation -project <id> -request-id <original-id>
steerline tests apply -project <id> -generation-id <original-id> -request-id <new-id> -expected-revision <n> -review-digest <sha256>
steerline tests stop-generation -project <id> -generation-id <original-id> -request-id <new-id>
```

生成复用 Runner generate/1 和 read_only。空模型结果、坏 ID、材料或目标变更不覆盖原文件；项目 tick 只保存原结果和已发生的应用事实，不写项目文件。查询返回只读 diff/review_digest；审阅后显式 apply，按当前 HEAD/分支、完整输入及目标原字节核对，取消后旧号不得新写。原 generate 重放不调用模型或应用；HEAD 前进后仍适用的输出可重新审阅再用新应用请求。作者维护 Plan 链接，正常提交/合入后检查并采用材料。

`cmd/test-feedback -level=L1/L2/L3/L4` 在原隔离证据目录追加 `managed-<id>.json`，保留原 summary、JSONL 与退出码。封套字段为 `id/sequence/selection/environment/definitions/before_commit/after_commit/before_clean/after_clean/results`；definitions[id] 是规范化单项 JSON 的 SHA256：字段固定按 `id,name,level,environment,command,check` 排序，非空 `note` 最后附加；以 UTF-8、无 BOM/缩进/末尾换行、与 Go `json.Marshal(Definition)` 相同的字符串转义编码（含 HTML 字符转义），不直接散列清单原始排版或属性顺序。results 中每项仅有 `test_id/status/message/log`，status 为 passed/failed/incomplete，log 指向同目录原 summary。定向 Go 子集和 build-only 不输出整体通过；gate-only 仅报告实际 controlled ID。

已接入文档/生成、程序报告、Runner R2 公开收集/读取/Git 观察、不可变持久化和只读执行页。运行内使用现有 `-out .steerrunner/artifacts/<唯一执行ID>` 新目录，并由项目维护忽略规则；测试入口最后原子发布 `managed-*.json`，同目录 `summary.json` 为首版保留的原始证据。默认系统临时目录不会自动成为 Run 产物。收集原请求先持久化，未知只查询原请求；报告按原候选/Run 保存并核验字节长度和 SHA256，UI 显示 Run/artifact/摘要引用。真实场景 `testing.management-local-codex` 的结果见[专属验收](acceptance/test-management.md)。干净同 commit 才能计入当前结果，脏或未知来源只作历史/待复验。

共享 Runner 的最小验证见[专属验收](acceptance/shared-runner-service.md)：固定 L1、请求/全局停止证明/reset 定向测试、新 L3 `runtime.shared-runner-projects` 与既有 waiting-merge-restart，以及新调试包的真实空闲服务拓扑。无模型，不操作安装环境。

架构解耦第五轮的最小验证见[验收与复现入口](acceptance/architecture-decoupling-round5.md)：宿主契约所有权/依赖闭包、checkpoint 双向投影隔离、并发写入下原实例/进程/build pin 同快照、原控制回执绑定、排空与幂等关闭、失败保锁和原 Runner 停服/结算证明。新增投影测试进入既有 quick-rules，所有权守卫进入 quick-execution-boundary；其余沿既有 SQLite、宿主和生命周期定向测试。补充 Windows 消费者与 Linux 宿主/进程包编译，不执行全量或真实模型。

[PR #350 复审](acceptance/architecture-decoupling-pr350-review.md)补齐公开 CLI 任务 DTO 解码回归和 Web 根包扩展后的闭包守卫反例夹具；集成主干候选安全停止后，定向覆盖停止/交付竞态、四轮读取与交付语义及受影响 E2E 消费者编译。证据和复现命令在专属验收记录，不运行全量或真实模型。

架构解耦第四轮的最小验证见[验收与复现入口](acceptance/architecture-decoupling-round4.md)：Task 交付证据纯校验、原子回滚/唯一交付/后继释放、原回执重放及身份冲突拒绝，另核对 Task 纯值与已收口文件的依赖守卫。纯模型测试沿 L1 quick-rules；新增服务重放用例纳入 quick-execution，所有权守卫纳入 quick-execution-boundary。最新主干集成后补测受影响的读取/投影和带标签消费者编译，不运行全量或真实模型。

架构解耦第三轮的最小验证见[验收与复现入口](acceptance/architecture-decoupling-round3.md)：会话原身份/游标/JSON 字节预算、正文摘要及 UTF-8 分页、能力字段投影、活动预算溢出零推进、同事务事件流与取消/异常关闭，以及 SQLite/存储端口禁止全部产品层依赖。新增查询测试沿 L1 quick-rules，真实 SQLite 读取/资源验证沿现有 SQLite 测试，守卫反例沿 quick-execution-boundary；不运行全量或真实模型。

架构解耦第二轮的最小验证见[验收与复现入口](acceptance/architecture-decoupling-round2.md)：历史/候选 DTO 纯投影、并发写入与事务结束后的原采样一致性、候选分页/预算、交付回滚/唯一交付/停止义务/后继释放，以及去掉 SQLite 查询层例外后的依赖守卫。新增查询测试沿 L1 quick-rules、现有读取矩阵沿 quick-owner-unified-startup、守卫反例沿 quick-execution-boundary；E2E 读取夹具同步事实字段，只补定向夹具测试和带标签编译，不以此声称完整 L2/L3 通过。

架构解耦第一轮的最小验证见[验收与复现入口](acceptance/architecture-decoupling-round1.md)：HTTP/Web 消费端生产依赖、taskrunner 禁止 SQL/具体存储/应用实现依赖、候选不存在与真实故障分类、首次领取及原请求重放。新增错误分类用例纳入 L1 quick-execution，SQLite 回归与守卫反例沿既有 quick-rules / quick-execution-boundary 执行；不要求全量或真实模型。

旧执行记录与空壳字段清理的验证范围见[Plan](plans/runtime-shell-cleanup.plan.md)与[验收](acceptance/runtime-shell-cleanup.md)：全源码引用审计、默认测试编译与固定 L1；不新增或改变分级场景，不用编译代替运行结果。

Runner 深 home 路径缺陷的最小验证见[短目录 Plan](plans/runner-short-state-path.plan.md)和[验收](acceptance/runner-short-state-path.md)：真实 Windows socket、命名空间/别名、caller 停止证明、controlled gate 及新调试包双项目/reset；不把模拟链当完整真实 L2。

执行域遗留清理（2026-10-16）按[Plan](plans/execution-legacy-cleanup.plan.md)核对全平台生产引用、生产入口编译和固定 L1；旧执行实现专属测试随实现删除，已无测试的 `execution/model` 不再独立登记 L1，仍被消费的值编解码和 caller/Task/存储/宿主边界测试保留。实际结果见[专属验收](acceptance/execution-legacy-cleanup.md)。

Runner 执行展示与最近消息的定向验证见 [Plan](plans/execution-observability.plan.md) / [验收](acceptance/execution-observability.md)，覆盖原子状态变化、只读消息契约、当前可见执行轮询与取消；不使用安装环境或真实模型。

三 EXE 安装部署与首次迁移的测试范围、命令和真实结果见 [部署 Plan](plans/install-deploy.plan.md) 与 [安装迁移验收](acceptance/install-cutover-20261015.md)。安装环境仅用于用户授权的实际部署操作，普通回归/模型验收仍使用下述隔离入口；空闲安装停启不代表模型执行或恢复通过。

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](architecture/current-code-architecture.md)。

SteerRunner 拆分后的测试归属及实施安排见 [Spec](spec/runner-test-convergence.spec.md)、[Plan](spec/runner-test-convergence.plan.md)和[专属验收](acceptance/runner-test-convergence.md)。SteerLine 保留调用方、Task/DAG、候选、交付、宿主和 Web 测试；Codex/Orca、原生恢复、沙箱及执行流程专项在 Runner 仓库维护。

完整 L2 保留真实 SteerLine → 固定 Runner EXE → 本机 Codex → 实现/报告/发布 → 真实 SteerlineTest PR → 合并 → TaskDelivery/后继释放。它覆盖 Runner 正常运行能力。执行专项迁出不降低这条链路为模拟测试。

## L1–L4 定义

SteerLine 直接采用 [开发项目测试架构规范](../SteerlineProjectGuideDoc/testing-architecture.md)。L1–L4 定义、L3 正交与并行设计、分级内容清单要求及跨项目测试归属统一在该规范维护；本文记录 SteerLine 的具体入口、环境、清单和实施缺口。

SteerLine 的 L1 是固定的快速集成/单元集合；完整 L2 是 controlled gate 加真实本机 Codex 核心交付链；L3 为等待合并时调用方重启、宿主生命周期、同 home 共享 Runner 和双 home 独立操作；L4 为构建/路径/宿主写入/同 ID 重放的系统边界。本次拆分和执行证据见 [Plan](plans/test-level-alignment.plan.md) / [验收](acceptance/test-level-alignment.md)。

SteerLine 的 Full 只运行现有确定性 Go 测试，不包含 L2/L3/L4/live。L2/L3/L4 的机器登记为 `tests/e2e/suites.json`，可读内容清单及本项目维护约定见 [E2E 指南](../tests/e2e/README.md#分级测试内容清单与维护)；实际结果在任务专属 acceptance 文档维护。

Web 候选停止参与修复的最小调用方/SQLite/HTTP/Web 验证见[Plan](plans/web-candidate-stop.plan.md)与[验收](acceptance/web-candidate-stop.md)，不启动安装现场或真实 worker。

## 日常选择

Web 历史启动请求误显示为候选的实例隔离回归见[专属验收 WCS-A5](acceptance/web-candidate-start.md#2026-10-21-历史请求误显示为候选修复)：使用定向 Web hook/执行页夹具验证占位、计数和名额，不启动真实 worker。

按变更运行最小必要范围，L1/Full 不自动运行真实模型。所有验收行为及实时结果在 [acceptance](acceptance/README.md)记录，原日志使用新建临时目录。

| 入口 | 范围 | 外部效果 |
| --- | --- | --- |
| L1 | 固定 caller/Task/存储/HTTP/边界，默认三个进程分组 | 无真实模型 |
| 定向 Go | 受影响的 owner 包或测试 | 普通用例为隔离 fixture |
| Full | 全部保留的确定性 Go 测试，现有三 worker 分区/覆盖核对 | 不自动运行 L2/L3/L4/live |
| L2 controlled | Runner 公开 CLI/PR 夹具驱动产品交付 | 无模型，仅 gate-only |
| 完整 L2 | controlled 通过后本机 Codex 真实三阶段及 GitHub PR/交付 | 消耗模型、创建并合并本次真实 PR |
| L3 controlled | 调用方重启、宿主生命周期、同 home 双项目共享 Runner、双 home 独立操作 | 无模型，独立临时 home/Git/SQLite，最多二场景并行 |
| L4 controlled | 构建/路径/SQLite 拒绝、宿主写入拒绝、同 ID 碰撞与重放 | 无模型，全部为隔离边界现场，runtime 最多二项并行 |
| Web | 前端单元、构建及按需浏览器场景 | 浏览器使用新建 simulate 包 |

```powershell
# 固定最小集；诊断串行可加 -workers=1
go run ./cmd/test-feedback -level=L1

# 定向，Go 参数放在 -- 后；runner 的 -out 等参数放在 -- 前
go run ./cmd/test-feedback -- ./internal/runnerclient ./internal/runnerbridge ./internal/taskrunner -count=1

# 只有确需全量时执行
go run ./cmd/test-feedback -workers=3

# 编译不等于场景通过
go run ./cmd/test-feedback -build-only
go test -tags=e2e ./tests/e2e/... -run '^$'
```

L1 固定范围以 [scoped.go](../cmd/test-feedback/scoped.go)及 [task_scope.go](../cmd/test-feedback/task_scope.go)为准；Full 保留原分区、包枚举与不遗漏/不重复核对。原生专项开关及其执行器不再属于 SteerLine 测试准入。

## L2 / L3 / L4 清单现状与入口

[注册表](../tests/e2e/suites.json)是当前可执行场景的来源，具体测试内容见 [维护清单](../tests/e2e/README.md#分级测试内容清单与维护)。当前核对结果：

| 级别 | 已登记内容 | 清单与实现现状 |
| --- | --- | --- |
| L2 | 2 项：controlled gate、live 本机 Codex | 有固定 ID、实际测试映射和选择校验，内容清单可追踪。 |
| L3 | 6 项：等待合并重启、正常宿主生命周期、同 home 共享 Runner、双 home 独立操作、审计 workflow 真实链、测试管理真实链 | controlled 支持多 suite/scenario；recovery/runtime 可跨包并行，总预算二。审计 live 精确选择 workflow.local-codex-audited，显式配置两个不同模型，不加入完整 L2。 |
| L4 | 4 项：源码构建保护、路径保护、宿主写入拒绝、同 ID 碰撞与重放 | 支持 `-level=L4 -suite=runtime -env=controlled`；每项独立资源，runtime 包内最多二项并行。 |

当前十二项场景有固定 ID、实际测试映射和分级清单。四项原 runtime ID 改为 L4，按旧 L3 选择会明确失败，不提供别名；拆出的正常流程使用新 ID。“固定清单”不等于已证明运行稳定，本次结果见 [分级验收](acceptance/test-level-alignment.md)，早期冻结版本见 [历史精简验收](acceptance/runner-test-convergence.md)。

```powershell
# 无 token 门槛，不能当作完整 L2
go run ./cmd/test-feedback -level=L2 -env=controlled

# 完整 L2，先固定独立 Runner EXE；不接受跳过 gate 直跑 live
$env:STEERRUNNER_EXE = 'C:/Tools/SteerRunner/steerrunner.exe'
go run ./cmd/test-feedback -level=L2

# Codex 工具进程内可使用既有独立进程上下文，保持同一 L2 与证据规则
./scripts/test-l2.ps1 -SteerRunnerExe 'C:/Tools/SteerRunner/steerrunner.exe'

# 四项受控流程；跨包/包内调度共用二并行预算
go run ./cmd/test-feedback -level=L3 '-suite=recovery,runtime' -env=controlled

# 四项系统边界；可用 -scenario=<逗号分隔ID> 精确选择
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled

# 工具进程有 Windows Job 约束时，使用独立同用户进程
./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -Scenario 'runtime.service-lifecycle,recovery.waiting-merge-restart'
./scripts/test-runtime-isolation.ps1 -Level L4
# 默认依次运行两级 runtime；分别生成报告
./scripts/test-runtime-isolation.ps1
```

L2 的 controlled 运行、证据或清理失败时，阻止真实阶段。仅 gate 报告 `coverage=gate-only`，完整两阶段才是 `full-L2`。退役的 delivery/recovery live、Orca chain、native 环境和旧 runtime 聚合 ID 明确拒绝，不映射到其他场景。

真实链仅用已登录本机 Codex、固定模型 `gpt-6-luna` / `medium`、固定公开 Runner profile 和已有 GitHub 权限。L2 使用其分级隔离夹具与 `erranliu/SteerlineTest` 本次专属 base/head；任务实现/业务 commit、push、PR 创建必须由真实 Runner/Codex 执行。驱动可以准备 base、查询、转 ready、精确合并与清理；不能代做任务伪造通过。

L2 通过证据包括公开阶段/final 来源、原 Run/候选、Git 提交中的真实产物、精确已合并 PR、唯一 TaskDelivery、后继 ready 且未启动、资源清理。SteerLine 不解析原生 rollout、统计 token 或重验沙箱。没有原生 usage 不是补造数值的理由；公开结果、真实 Git/PR 与 Task 证据缺失仍必须失败。详见 [E2E 指南](../tests/e2e/README.md)。

## 夹具、Web 与调试包

确定性测试使用公开 Runner CLI 子进程夹具，不要求安装真实 Runner、Codex、Orca 或远端主机。夹具仅提供脚本化公开回执/Run/事件，不执行或模拟原生 workflow。原请求、未知回执、授权 revision、投影/游标、绑定、事务回滚与停止义务由对应 caller/SQLite/HTTP owner 验证。

前端与 HTTP owner 使用 `scripts/test-web.ps1`，浏览器入口及新包前置见 [Web 工程](../web/README.md)。浏览器 fixture 消费公开 Run/请求证据，浏览器关闭不停止原业务、同请求不重复提交、多项目/多标签隔离等要求继续保留。

临时/AI 调试及真实 PR 调试先运行 `scripts/new-debug-release.ps1`，创建 `C:\SteerlineDebug` 下独立环境；不使用安装现场。生成默认 live，AI 明确 `--simulate`；模拟方案使用包内公开 Runner CLI fixture，真实方案使用固定真实 EXE。`scripts/test-debug-release.ps1 -SteerRunnerExe <absolute>` 验证包生成、冻结、停止与 reset；其 `-Live` 复用完整 L2，不再运行第二条沙箱验收链。详见 [调试环境验收](acceptance/debug-environment.md)。

认证引用既有宿主登录，不复制安装数据库、认证或用户配置。方案启动后冻结，换方案新建包；重置不备份且活动服务/资源阻止重置。

## 报告与文档检查

runner 默认写系统临时目录 `steerline-test-feedback/<本次时间>`；显式 `-out` 须为新空目录。核对实际源码/dirty、选择范围、用例终态、`passed`/`complete`、场景/清理和覆盖类型；skip、零匹配、超时、缺证据都不是通过。失败保留原诊断，不自动重跑；原生未确认停止时保留现场，不用父进程退出代替停止事实。

纯文档检查：

```powershell
python scripts/check_doc_links.py
git diff --check
```

新 Plan 的测试计划按任务专属验收 ID 映射场景/命令、环境、时点、通过条件与证据位置；缺能力须列入实施或明确前置。结果只登记 acceptance，Plan 不维护实时结果。历史原生结果仅证明各自冻结版本与原范围，参见 [历史验收](acceptance/archive/README.md)。
