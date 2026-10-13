# 测试总入口

当前产品入口为本地 Codex 与 Orca 单终端 `host=orca` / `orca_terminal_v3`（local/environment），supervised 已退役。当前通信及验证范围见[单终端验收](acceptance/orca-terminal-transport.md)与[真实调试记录](acceptance/orca-terminal-remote-retest-2026-10-10.md)：本机有限样本三阶段交付通过，本机最终 draft 保护未重新跑真机；远端升级 CLI 且预先确认目录信任后有限样本闭环通过，首次信任阻塞后原任务自动继续仍未通过。两目标受控回归不代替远端真机。旧单轮远端场景仍已退役，正式注册表不含新版远端交付；旧 supervised、统一模板及单消息接线证据保留各自历史范围。

从仓库根目录运行以下命令。日常按改动选择最小必要范围；需要完整基线时选择 Full，涉及原生交付或恢复时选择对应 L2/L3。各入口分别执行，不会因为跑了 L1 或 Full 就自动跑真实模型。

本页维护测试方法/级别和工具入口；所有自动/人工验收行为及已验/待验记录集中于[acceptance](acceptance/README.md)，任务 Spec/Plan 链接对应验收项。自动验收可独立成任务或随实现执行，人工为主的验收活动不进执行 DAG；少量允许后置的人工项不阻碍无关后续执行，仍由 execution-map 链接保留待验事实。原始测试日志写临时目录，仓库只保存最小结果摘要。

## 文档静态检查

实现 Plan 的“测试计划”须覆盖任务范围内全部自动验收项，按 ID 链接任务专属 acceptance 文档，说明场景/命令、环境、时点、通过条件及证据登记位置。一个场景可覆盖多个项；必要回归另标，不将整组测试一概算作产品验收。缺工具/驱动/环境时规划最小补齐步骤或明确前置，草稿未定内容在准入前固定；实际验收前落实可用入口。人工项单列窗口和阻塞范围。纯文档修订的静态检查计划不能替代尚待实施任务的产品验收计划。

纯文档、目录导航或源码注释变更不运行产品/模型测试。使用以下独立入口检查本地导航、任务身份和差异格式：

```powershell
python scripts/check_doc_links.py
python scripts/roadmap_task_id.py check
git diff --check
```

链接检查覆盖仓库 README 与 docs 下的 Markdown 文件/目录目标，跳过代码示例、外部 URL 和页内锚点；不声明外部页面、历史章节锚点或原实验可重放性已经验证。任务身份检查使用当前规划，不复用历史验收中写死的任务集合。移动原始证据或原型时另行核对字节摘要；固定 commit 中的路径与契约摘要不因当前导航改写。

## 选择范围

| 入口 | 用途 | 模型 token / 外部效果 |
| --- | --- | --- |
| L1 | 固定领取/事务/SQLite 接线、规则和依赖边界；默认三个独立进程并发，可追加定向用例 | 默认无真实模型 |
| 定向 Go 回归 | 指定包或测试，定位受影响行为 | 普通用例无真实模型；显式原生验收另行配置 |
| Full | 全部未启用 e2e tag 的确定性 Go 回归，包含受控进程、Git、SQLite 和接线测试 | 默认无真实模型；登记的原生 opt-in 用例会跳过 |
| L2 controlled | 只检查 smoke 的无 token 门槛 | 本地隔离项目与 loopback Git/PR provider；不构成完整 L2 |
| 完整 L2 | 门槛通过后，本地 Codex 真实交付链路 | 消耗 token；在 SteerlineTest 创建、转 ready、合并真实 PR，并清理本次分支 |
| L3 controlled | 独立选择交付、等待合并后重启等受控场景 | 无 token；本地隔离项目与 loopback provider |
| L3 live | 本地 Codex 交付与恢复 | 消耗 token；使用 loopback provider |
| L3 native | Orca 工作区删除与重启 | 无模型；创建、删除本次隔离工作区 |
| E2E 基础设施 | 测试环境、driver、codec、证据和清理 | 无真实模型 |
| 编译检查 | 检查包与带 tag 场景能否编译 | 不执行场景，不构成测试通过证据 |

Full 将 `internal/execution` 和 `tests/integration` 两个业务包分别按顶层用例划为三组。每个 worker 顺序运行自己的 execution 分区和 integration 分区；另一个独立串行 Go 命令完整运行其余包（`-p=1 -parallel=1`，包括 `internal/app/host` 服务宿主）。最多四组命令并发，各包使用独立测试进程和产物目录，总期限为 75 分钟，各组命令仍保留原期限。报告的 `core_package`、`integration_package` 和两份用例枚举记录实际所有者，并核对各顶层用例和其他包均不遗漏、不重复。它与 L3 的“多组端到端场景”是两个范围；完整项目验收应明确选择哪些入口，而不是把它们都称为 Full。

## L1、定向与 Full

```powershell
# 日常最小入口（-quick 是相同入口的旧命令名）
go run ./cmd/test-feedback -level=L1

# 同一固定范围串行执行，用于诊断并发或资源问题
go run ./cmd/test-feedback -level=L1 -workers=1

# L1 加本次改动的定向测试；Go 参数放在 -- 后
go run ./cmd/test-feedback -level=L1 -- ./cmd/test-feedback -run '^TestLevelSelectionsRejectAmbiguity$' -count=1

# 只跑指定包，并保存报告
go run ./cmd/test-feedback -- ./internal/console -count=1

# 完整确定性回归：三 worker 入口
go run ./cmd/test-feedback -workers=3

# 完整串行入口：用于定位进程分组相关问题
go run ./cmd/test-feedback -workers=1
```

L1 的固定选择以 [quickTests / quickPackages](../cmd/test-feedback/scoped.go) 与 [任务包选择](../cmd/test-feedback/task_scope.go) 为准；定向用例来自各包的 `*_test.go`。Full 会枚举并核对包/核心顶层用例，不能用部分通过替代完整覆盖。三 worker 入口拒绝已启用的原生验收开关；原生验收按下面的独立入口选择。

L1/quick 默认 `-workers=3`，最多三个分组命令并发；各分组保持 `-p=1 -parallel=1`，使用独立 home、临时目录、日志和子进程所有权。显式 `-workers=1` 使用相同固定范围串行运行。Full 的 `-workers=3` 仍采用上面的完整分区策略。

固定集成组保留 `TestCD67CoreBoundary`、`TestTM04ExecutionWiring/claim_transaction_failure_and_original_receipt` 与 `TestTM07StorageWiring`；Go 的分层 `-run` 精确排除长恢复及重复双回合流程。定向追加始终保留固定范围，包括追加 `tests/integration` 时。重启、回执丢失、多回合、候选调度、复杂远端协议、重服务查询按受影响实现选择；Full 保留全部测试。

```powershell
# 修改 Codex 会话/回执接线时，只追加一处双回合验证
go run ./cmd/test-feedback -level=L1 -- ./tests/integration -run '^TestTM04ExecutionWiring$/^intent_binding_and_two_original_turns$' -count=1

# 修改 Orca 原输入恢复时，追加未知提交只观察原终端的回归
go run ./cmd/test-feedback -level=L1 -- ./internal/execution/adapters/orcatransport -run '^TestTerminalUnknownSubmissionOnlyObservesOriginal$' -count=1

# 修改候选调度时，追加被影响的两个回归
go run ./cmd/test-feedback -level=L1 -- ./internal/execution -run '^TestCandidate(OpenPRPollPreservesOriginalPendingInput|ResumeErrorDoesNotStarveSibling)$' -count=1
```

宿主控制/心跳/退出、原生项目初始化、身份/权限冻结、工作区及发布契约仍在固定 L1；定向回归不构成真实模型验收。此前完整 L1 的长流程耗时不能直接当作新版固定范围的性能基线，比较时同时核对选择范围。

## L2 与 L3

L2/L3 当前在 Windows 上串行运行。controlled 使用真实 SteerLine、Git、SQLite 和受控 worker/provider；当前登记的 live 使用真实 Codex。真实场景固定 `gpt-6-luna` / `medium`，采用当前 Windows 原生方案：`workspace-write`、`unelevated`、`on-request`、`auto_review` 和网络访问，核对冻结身份及精确写根。

本地 live 需要已登录的 `codex-cli 0.159.2`；完整 L2 需要 [SteerlineTest](https://github.com/erranliu/SteerlineTest) 的写权限。场景使用隔离测试项目；本地不需要 elevated sandbox 管理员初始化。具体环境准备、权限断言、usage 和资源所有权见 [E2E 环境指南](../tests/e2e/README.md)。

```powershell
# 固定 L2：先 controlled，再本地真实链路；门槛失败不进入真实阶段
go run ./cmd/test-feedback -level=L2

# 只诊断门槛，无 token，报告为 gate-only
go run ./cmd/test-feedback -level=L2 -env=controlled

# L3 的 controlled 与 live 分开选择
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery,runtime' -env=controlled

# 每个真实场景可以独立运行
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex'
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.local-codex'
go run ./cmd/test-feedback -level=L3 -suite=workspace -env=native

# 独立 debug 包中的双任务真实沙箱与 Git helper 发布
go run ./cmd/test-feedback -level=L3 -suite=runtime -env=live '-scenario=runtime.sandbox-git.local-codex'
```

| 级别 | 环境 | 组 | 场景 ID |
| --- | --- | --- | --- |
| L2 | controlled | smoke | `smoke.delivery` |
| L2 | live | smoke | `smoke.local-codex` |
| L3 | controlled | delivery | `delivery.basic` |
| L3 | controlled | recovery | `recovery.waiting-merge-restart` |
| L3 | controlled | runtime | `runtime.installed-debug-isolation` |
| L3 | live | delivery | `delivery.local-codex` |
| L3 | live | recovery | `recovery.local-codex` |
| L3 | native | workspace | `workspace.orca-removal-restart` |
| L3 | live | runtime | `runtime.sandbox-git.local-codex` |

注册表 [suites.json](../tests/e2e/suites.json) 是场景 ID、包、测试名和环境的来源。L3 的 `-suite` / `-scenario` 支持逗号多选；省略场景 ID 时运行所选组和环境下的全部场景。PowerShell 中含点的场景参数应整体加引号。L2 是固定选择，不接受按场景挑选或直接选择 live；L2/L3 不与 `-workers`、`-quick`、`-build-only` 或原始 Go 参数混用。

## 基础设施与编译

```powershell
# runner / 测试适配层的定向回归
go test ./cmd/test-feedback ./tests/e2e/internal/... -count=1

# 含 e2e tag 的项目隔离、部分准备失败和清理回归
go test -tags=e2e ./tests/e2e/internal/... ./tests/e2e/cmd/... -count=1

# 依赖边界：生产不能依赖测试工具，场景通过测试适配层访问实现
go test ./internal/testkit/boundary/... -count=1

# 普通 Go 包编译检查
go run ./cmd/test-feedback -build-only

# 只编译全部 E2E 场景，不运行场景
go test -tags=e2e ./tests/e2e/... -run '^$' -count=1
```

带 tag 的场景包应通过分级 runner 执行；直接运行缺少 runner 环境的场景会失败。编译成功不证明业务断言、原生证据或清理通过。

## WinUI 历史验证与当前 TUI 改造

#44 已退役为历史，代码删除由 #91 实施；下面命令仅用于原构建取证，不是当前任务要求。当前 TUI/HTTP 的新增测试/驱动与最小回归以[#91 Plan](spec/tui-http-client.plan.md)为准：

```powershell
dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj -c Release
dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug
```

单元测试验证客户端核心逻辑；工程编译不证明真实窗口操作已经验收，人工项见[WinUI 验收](acceptance/archive/superseded/phase2-winui-foundation.md)。

独立调试自动入口使用生成的 `C:\SteerlineDebug` 包和 SteerlineTest，不能使用安装环境。生成/模拟/隔离/重置与脚本 PR 分别由 `scripts/test-debug-release.ps1`、其 `-Live` 参数验证；真实 Codex 沙箱与 Git 工具回归已登记为上面的 L3 `runtime.sandbox-git.local-codex`；`scripts/test-codex-sandbox.ps1` 保留独立入口，复用相同双任务驱动。各入口的前置、外部效果和证据见[调试环境验收](acceptance/debug-environment.md)与[沙箱验收](acceptance/codex-sandbox-git.md)。

## 独立原生验收

这些检查绑定专门的原生现场，不属于固定 L2/L3 注册表。先阅读各测试的前置字段、原身份和实际效果，再只运行选定测试；不要批量开启所有 `STEERLINE_*` 开关。未配置而跳过，表示未验收。旧 Orca 测试已经随旧 adapter 删除，不能在当前源码执行；当前单终端原生范围见上方专属验收。

| 检查 | 前置与范围 | 测试来源 |
| --- | --- | --- |
| 原运行恢复预览 | `STEERLINE_REAL_RECOVERY_CONFIG` / `STEERLINE_REAL_RECOVERY_RUN_ID`；读取隔离原现场 | [TestRealIsolatedRecoveryPreview](../internal/app/host/recovery_real_probe_test.go) |
| Windows 服务窗口 / Ctrl+C helper | 只由绑定原进程、启动回执和报告路径的专门探针调用 | [Windows 平台测试](../internal/app/host/platform_windows_test.go) |
| elevated sandbox 凭据检查与设置 | 为显式需要 elevated 的开发场景使用；无 token 的检查和可能触发 UAC 的初始化是两步 | [E2E 指南的 sandbox 工具](../tests/e2e/README.md#真实环境)、[设置脚本](../scripts/setup-e2e-sandbox.ps1) |

已删除的旧 Orca host-wait 与 supervised P10 探针只保留[固定源码历史](https://github.com/erranliu/SteerLine/tree/872dba4462a4f659d7685eb2a397211a1653c632/internal/execution/adapters)，不能在当前包路径执行。当前原生检查必须按现存源码准备隔离现场；其他 opt-in 测试的 skip 不算通过。

## 产物与判断

所有 `cmd/test-feedback` 入口（L1、Full、L2/L3、串行、定向、编译和日志汇总）默认将证据写到系统临时目录 `steerline-test-feedback/<运行时间>`。可为每次运行显式指定新的 `-out`，已有目录中的产物不能覆盖。

```powershell
$runRoot = Join-Path $env:TEMP ('steerline-tests-' + (Get-Date -Format yyyyMMdd-HHmmssfff))
go run ./cmd/test-feedback -level=L1 ('-out=' + (Join-Path $runRoot 'l1'))
```

查看退出码和 `summary.json`：确定性入口需要 `passed=true`、`complete=true`，并核对 scope、源码/工作树、命令、枚举与实际终态。E2E 需要选择范围、coverage、每个 scenario 的通过/清理和原生证据都符合要求；`gate-only` 不能当完整 L2。真实运行还需正数、同身份及同模型的 usage；recovery 不应增加模型回合。父子测试计数及时间会重叠，不能当成独立场景数量或累加为总耗时。

L1 报告的 `workers`、各组 `started_at` / `finished_at` 和 `wall_seconds` 可核对实际并发；总耗时使用准备加执行时间，并行分组时间不能求和作为墙钟时间。

失败、超时、证据/清理缺失、零匹配或报告不完整都不能算通过；runner 不自动重试。重新运行使用新目录和新现场，保留前次失败。每次原始报告和失败诊断保留在本次输出目录；仓库只提交最小验收摘要和仍必要的结果索引，避免提交完整日志、认证、数据库、原始 prompt 或 dispatch capability。测试样本维护在对应包的 testdata。

历史结果只证明报告中记录的候选版本。[最近完整重跑](acceptance/archive/completed/all-tests-after-288-2026-10-10.md)注明逐次源码、skip 与原生自动审核拒绝，不宣称所有入口通过，也不能替代后续改动的验证。此前保留的[历史基线](acceptance/archive/completed/latest-baseline.md)和更早[固定 Git 快照](acceptance/archive/completed/history.md#历史取证)继续按原范围读取。

## 扩展与 Plan

新增 E2E 场景、共享环境和测试依赖边界的约定统一见 [E2E 扩展指南](../tests/e2e/README.md#扩展与-plan)。普通包测试放在相应 `*_test.go`；L1 固定范围和 Full 调度见 [scoped.go](../cmd/test-feedback/scoped.go)，选择参数见 [main.go](../cmd/test-feedback/main.go)。

新写或修改的实现 Plan 应列出最小必要的测试级别、组/场景 ID、环境、命令、时点、通过条件及证据位置；未选择相关组时说明原因。纯文档改动检查链接、命令和场景注册一致性即可，不默认要求 L1、Full 或付费场景。

[分级体系 Spec](archive/spec/completed/tiered-e2e-testing.spec.md) · [真实模型接入 Spec](archive/spec/completed/live-tiered-e2e.spec.md) · [依赖边界 Spec](archive/spec/completed/e2e-dependency-boundaries.spec.md)

固定 L1 已包含 task/model 与 task 应用测试；历史迁包映射从固定 Git 快照取证，不默认启用 Full/L2/L3/live。

公共测试适配器位于 `internal/testkit/fixtures`，仅允许 execution 与 app/host 的测试文件直接引用；生产依赖、其他测试所有者和经 helper 的间接引用仍受边界守卫约束。同一业务场景只保留一个执行入口，历史任务的聚合测试不再重复调用已被 Go 自动发现的顶层测试。双回合接线保留在 `TestTM04ExecutionWiring/intent_binding_and_two_original_turns`；Orca 未知提交后的原终端观察恢复保留在上面的定向入口。
