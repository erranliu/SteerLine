# 测试总入口

执行页用量与费用接入的最小验证见[Plan](plans/run-usage-cost-ui.plan.md)和[验收](acceptance/run-usage-cost-ui.md)：CLI 原实例/快照绑定、调用方候选/目录身份与费用隔离、HTTP 只读边界、Web 部署字段保存纳入既有 L1 owner；前端组件定向验证部分/完整/未知/零、精度、查询取消及刷新。原生用量采集与计价矩阵继续由 Runner 所有，不新增完整执行场景。

项目维护一键生成/全局 generate 队列的定向 L1 见 [Plan](plans/generate-queue.plan.md) 和 [验收](acceptance/generate-queue.md)。应用批量/进度/排队接管反例登记在 `quick-owner-generate-queue`；生成规则与 SQLite 留在既有规则包，配置保存归 `quick-owner-web-config`，未新增 E2E 场景或扩大 full。

可靠性交付简化的本批定向验证见[实施 Plan](plans/reliability-simplification.plan.md)和[验收](acceptance/reliability-simplification.md)：Go 部署目标/一次恢复、实际命令完整绑定、同快照观察与 CAS；新用例进入既有 L1 owner 分区，公开 CLI 联验另用实际固定 Runner，未运行完整 L2/L3 或安装环境。

部署中断自动恢复的最小验证见[Plan](plans/deployment-interrupted-recovery.plan.md)与[验收](acceptance/deployment-interrupted-recovery.md)：固定 L1 `quick-owner-deployment-recovery` 验证缺失/陈旧回执的恢复准入、readiness、新停机证明及失败围栏；CLI 参数归 `quick-owner-runner-preflight-cli`。脚本路由另运行 `scripts/test-deployment-stop.ps1`，不使用安装现场或真实 OS 重启。

近期合入 PR #352–#365 的补验和人工操作见[本轮导航](acceptance/merged-pr-regression-20261020.md)。固定 L1 将累积的应用启动测试拆为 `quick-owner-runner-startup`（接管、启动撤销与共享 Runner）和 `quick-owner-unified-startup`（项目/宿主生命周期），保留全部原测试及每组原三分钟预算；新增无法解析项目 census 时零共享服务启动的反例。覆盖检查按固定分区核对包终态，并拒绝跨 owner 重复执行，不用重复一组补缺失组。

调用方边界收口（2026-10-20）的最小验证见[Plan](plans/architecture-boundary-consolidation.plan.md)和[专属验收](acceptance/architecture-boundary-consolidation.md)：新 owner 依赖闭包、启动/正常 stop 共用 outbox、takeover 公开校验与原事实匹配、暂存部署 CLI 在线预检；只跑隔离定向测试，不使用安装现场或完整原生流程。

Runner 兼容构建接管的定向测试与未验边界见[专属验收](acceptance/service-generation-recovery.md)：公开 takeover/startup stops、当前 pin 与历史 candidate/Check 分离、启动撤销 outbox、兼容部署证据；不运行安装环境或以 fixture 代替原生资格。

Windows 启动闪窗修复的最小验证见[专属验收](acceptance/startup-console-windows.md)：固定 L1 `quick-core` 的 `TestBackgroundCommandsNoConsole` 从 detached 宿主核验四类后台命令的实际控制台句柄；另定向回归统一启动与共享 Runner，不运行全量或使用安装环境。

## 测试管理文档与结果

[项目更新与候选版本收敛](spec/project-maintenance-simplification.spec.md)按[Plan](plans/project-maintenance-simplification.plan.md#测试计划)实施，实际结果与未验边界见[专属验收](acceptance/project-maintenance-simplification.md)。系统在宿主观察/检查及新候选准备时自动读取远端 Plan、规范与清单，生成结果保存在项目库，不再人工审阅/apply 或提交旁置安排文件。每个新候选独立固定最新代码及材料，过期候选只读测试历史，PR仍可交付。HTTP GET和前端轮询只读；失败显式重试，未知只核对原请求。维护规范、清单及实际测试程序仍由开发维护。

L1 新规则登记原 project-maintenance、test-management、app/SQLite/taskrunner owner；L3 扩展既有 `runtime.project-maintenance` 与 `testing.management-local-codex`，不另建业务链。上一批结果保留于[原验收](acceptance/project-maintenance.md)。

可映射到执行页的清单为 [test-list.json](testing/test-list.json)，格式遵循[测试管理 Spec](spec/test-management.spec.md)。每项必需 `id/name/level/environment/command/check`，仅 `note` 可选；拒绝未知字段、重复键/ID、空必需值与非法级别。首批登记固定 L1 整体及现存十三项 E2E 场景。E2E 执行选择仍以 `tests/e2e/suites.json` 为准；新增、删除、改级/环境/入口/主要断言时，依据通用规范同步修改实现、执行登记、展示清单和受影响 Plan。Web/脚本入口暂未登记，不推断它们已被覆盖。

系统测试安排由远端固定版本的 Plan、`docs/testing.md` 与 `docs/testing/test-list.json` 自动生成，仅保存在项目库。无需自动测试时，Plan必须明确说明原因，才能接受生成的空数组。无效输出或缺失定义阻断新候选，已有候选及其PR继续按原快照执行。

原生成查询与停止继续使用正式 CLI；生成、重试及一次性本机 Codex profile 设置由项目维护流程消费同一服务。原请求重放只对账，不重新调用模型。

```powershell
steerline tests generation -project <id> -request-id <original-id>
steerline tests stop-generation -project <id> -generation-id <original-id> -request-id <new-id>
```

候选通过原执行提示中的系统安排，以及固定base worktree内的真实规范/清单路径取得测试要求，测试程序必须实际执行并产生受管报告。页面不能以模型 final 自述通过。过期候选停止发起新 collect，已有证据只作历史；当前性未知时不显示当前通过。

Web 执行配置的最小验证见[Plan](spec/web-execution-config.plan.md) / [验收](acceptance/web-execution-config.md)：Web 配置/目录 owner 纳入固定 L1，`configuration.execution-profiles` 为独立 L3 受控保存、正式重启及关闭 Web 后 CLI 消费场景。真实目录专项内部先生成新 debug 包，仅查询 Codex/本机 Orca/已配对 Orca，不计完整执行链验收。

`cmd/test-feedback -level=L1/L2/L3/L4` 在原隔离证据目录追加 `managed-<id>.json`，保留原 summary、JSONL 与退出码。每报告对应一个登记测试集/环境；执行前冻结 32 位随机 `id`、`started_at`、`definition`、`selection/definitions`、实际 `command`、`repository`（Git common dir）、`branch` 和可选 `rerun_of`；执行后补 `finished_at/sequence/before_commit/after_commit/before_clean/after_clean/results/evidence`。`evidence` 声明同目录 `summary.json` 的原字节数/SHA256。definitions[id] 是规范化单项 JSON 的 SHA256：字段固定按 `id,name,level,environment,command,check` 排序，非空 `note` 最后附加；以 UTF-8、无 BOM/缩进/末尾换行、与 Go `json.Marshal(Definition)` 相同的字符串转义编码（含 HTML 字符转义），不直接散列清单原始排版或属性顺序。results 中每项为 `test_id/status/message/log`，status 为 passed/failed/incomplete，log 指向同目录原 summary。仅已有稳定子项的入口使用可选 `scope`，不跨报告拼接通过。定向 Go 子集和 build-only 不输出整体通过；gate-only 仅报告实际 controlled ID。

测试管理为项目级只读页面 `/projects/:id/tests`。脚本可独立登记，无需 Task/Run；候选仍从原 EvidenceView 核验，不用独立历史代替候选证明。新增/补登记复用 `STEERLINE_HOME` 发现应用：

```powershell
# 实际完成所选测试后可自动登记；登记失败不改变原测试退出码
go run ./cmd/test-feedback -level L1 -record-project <配置ID> -out <新的绝对输出目录>
# 单测试集的显式复验，父报告须已登记；无映射为独立运行
go run ./cmd/test-feedback -level L3 -suite testing -scenario 'testing.records' -env controlled -record-project <配置ID> -rerun-of 'testing.records=<原报告ID>' -out <新目录>
# 仅补登记现有报告，不运行测试
steerline tests register -project <配置ID或项目ID> -report-dir <报告绝对目录>
```

报告至多 128 KiB，summary 至多 512 KiB，整份 JSON/base64 命令至多 1 MiB；逐报告原子登记并以项目+报告 ID 幂等。目录内普通文件才可读取，链接越界、内容/摘要冲突拒绝；缺失/过大 summary 仍登记原始结果并标证据不可用，不计复验通过，可按原声明摘要补齐。`registration.json` 逐次追加报告 ID、传输请求号、登记/拒绝/未知状态及诊断；发送前先保存未知状态，补登保留既有尝试。离线保留原产物，用原报告补登；本地 register 有失败/未知项非零，页面不提供执行或改结果按钮。详情接口将可能超过浏览器安全整数范围的 producer `sequence` 作为字符串输出，原报告字节/数值不变。复验只认显式链最新一份可比且完整覆盖原失败范围的通过报告，原失败保留；源码脏/未知允许展示复验通过，但显著标记“源码未固定”，不能当作固定 SHA 证明。

已接入文档/生成、程序报告、Runner R2 公开收集/读取/Git 观察、不可变持久化和只读执行页。运行内使用现有 `-out .steerrunner/artifacts/<唯一执行ID>` 新目录，并由项目维护忽略规则；测试入口最后原子发布 `managed-*.json`，同目录 `summary.json` 为首版保留的原始证据。默认系统临时目录不会自动成为 Run 产物。收集原请求先持久化，未知只查询原请求；报告按原候选/Run 保存并核验字节长度和 SHA256，UI 显示 Run/artifact/摘要引用。真实场景 `testing.management-local-codex` 的结果见[专属验收](acceptance/test-management.md)。原候选的当前结果仍要求干净同 commit；项目历史按上文独立展示源码状态和显式复验。

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

临时/AI 调试及真实 PR 调试必须先读取并遵循 [独立调试流程](runtime-install-debug.md)：统一生成、执行配置复制源、方案冻结、专属 SteerlineTest base/head 与无备份重置均在该文档维护。不得使用、修改或重置 `C:\SteerlineInstall` 或保留的 `C:\SteerlineRuntime`，不得复制安装版数据库、认证或配置准备现场。既有分级回归继续使用自身隔离夹具。

`scripts/test-debug-release.ps1 -SteerRunnerExe <absolute>` 验证包生成、冻结、停止与 reset；其 `-Live` 复用完整 L2，不再运行第二条沙箱验收链。详见 [调试环境验收](acceptance/debug-environment.md)。

## 报告与文档检查

runner 默认写系统临时目录 `steerline-test-feedback/<本次时间>`；显式 `-out` 须为新空目录。核对实际源码/dirty、选择范围、用例终态、`passed`/`complete`、场景/清理和覆盖类型；skip、零匹配、超时、缺证据都不是通过。失败保留原诊断，不自动重跑；原生未确认停止时保留现场，不用父进程退出代替停止事实。

纯文档检查：

```powershell
python scripts/check_doc_links.py
git diff --check
```

历史原生结果仅证明各自冻结版本与原范围，参见 [历史验收](acceptance/archive/README.md)。

## Spec / Plan 的验证安排

Spec 的验证需求及 Plan 的测试计划遵循上文采用的测试架构规范，以最小必要范围验证本次变更。新编写或本次修改的实现 Plan 必须包含“测试计划”，覆盖任务范围内全部自动验收项，按任务专属验收项 ID 映射测试级别、组/场景、环境、命令、执行时点、通过条件与证据登记位置。同一场景可覆盖多项；未选择相关组时说明原因，不默认运行 L2/L3 或 Full。

需要新增场景、工具、驱动或环境能力时，纳入实施步骤或明确前置，不只写“后续补验”。草稿可标待定；纳入 roadmap 执行前明确验证方式和能力补齐安排，实际验收前落实可用入口。分级入口与扩展约定见 [E2E 指南](../tests/e2e/README.md)。

Plan 安排执行自动验收、更新任务专属 acceptance 明细；roadmap 任务另同步 execution-map 汇总。Plan 不维护实时结果。人工项单列参与窗口和阻塞范围，允许后置项不阻碍无关后续执行；具体文档归属、验收分类及记录要求统一见 [验收流程](acceptance/README.md)。
