# 应用与项目生命周期收拢验收

归属：2026-10-16 普通开发设计，[Spec](../../spec/application-project-lifecycle.spec.md)。仅为本项收拢维护验收，不覆盖先前统一启动、执行域删除或 Runner 原生专项。

当前状态：2026-10-16 已按审计收窄并实施；[实现 Plan](../../plans/application-project-lifecycle.plan.md)覆盖全部自动项。定向、L1、Windows/Linux 编译、部署脚本及选定 L3/L4 已通过；最终补入未知启动用例后的 L1 复验同样通过，ALC-A1–A6 全部通过。

## 自动验收安排

| ID | 级别 / 组或场景 | 环境、入口与能力 | 通过条件与证据 | 状态 |
| --- | --- | --- | --- | --- |
| ALC-A1 | 源码边界、确定性编译 | 全平台/build tag/脚本引用核对；`go build ./...`、Linux 生产交叉编译、`go run ./cmd/test-feedback -build-only`；现有 `internal/testkit/boundary` 与 app client 边界测试 | 唯一生命周期 owner、旧入口删除、只读零启动、全入口编译；保存删除清单与 runner 编译/边界报告 | 通过 |
| ALC-A2 | L1 owner / project resource 取得释放 | 隔离 temp home/store/公开 CLI 夹具；在 app 增加资源取得点失败、部分启动、调用排空、重复关闭、本地关闭失败保留锁用例；借用 store 不关闭的断言放原 client/store 测试 | 每项资源一个最终关闭者，失败保留部分事实，本地/Runner 结论分开；定向命令和证据由实现 Plan 固定 | 通过 |
| ALC-A3 | L1 owner / actor 与 Runner 边界 | 复用 taskrunner、runnerbridge、runnerclient、SQLite 的原请求/结算测试；迁移原 shutdown 测试至项目资源 owner，删除退役的逐项目停止回执接线与专属测试，补 ready/stop/请求竞争及最终 checkpoint 与借用 store | 原停机 request_id/重试语义保持、最终证明原身份匹配、原请求及未知事实不改绑；保存原 request/body、计数和 checkpoint/回执证据 | 通过 |
| ALC-A4 | L1 owner / aggregate shutdown | 扩展 `TestUnifiedStartupLifecycleSharedPIDIndependentInstances`、`TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost` 相关夹具；补兄弟部分失败、Close-before-Start、缺结果及收尾等待取消 | 单项目不等 PID、兄弟不跳过、全部门槛先关闭、空值不冒充成功；保存逐项目事实和宿主结果 | 通过 |
| ALC-A5 | L1 owner / stop proof、部署受控脚本 | 复用 `TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite`、`TestDeploymentCandidateVerificationKeepsOriginalBinding` 和 `scripts/test-deployment-stop.ps1`；补纯 catalog 与共享校验反例 | 原失败、配置/DB不变，身份/成员/锁/证明错配拒绝，未启服务；保存原文件 hash 与公开 CLI 调用记录 | 通过 |
| ALC-A6 | L3 `runtime.service-lifecycle`、`recovery.waiting-merge-restart`；L4 `runtime.service-write-isolation` | 既有隔离 controlled 场景；命令见下文，复用现有 Windows 独立进程入口，不使用安装目录 | 选定三场景业务/证据/清理通过，原候选/请求不重复，跨 home 互斥与旧实例拒绝；保留 summary.json / scenario.json | 通过 |

L1 及定向 owner 命令在实现 Plan 固定，至少覆盖 app、taskrunner、runnerclient、runnerbridge、SQLite、共享请求门槛和仍有实际改动的 host/localservice；不要求每个包单独新增测试，不将零匹配计通过。

ALC-A6 复用现有选择：

```powershell
go run ./cmd/test-feedback -level=L3 '-suite=recovery,runtime' -env=controlled '-scenario=runtime.service-lifecycle,recovery.waiting-merge-restart'
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario=runtime.service-write-isolation
```

Windows 工具 Job 约束下，使用现有独立进程入口执行同一选择；`scripts/test-runtime-isolation.ps1` 已补齐 Scenario 参数及独立进程透传，执行了同一选择，未切换安装现场或以更小假场景替代。不新增长期场景 ID；若实施改变既有场景主要断言，同步实现、机器登记、可读清单及选择校验。

所有原始报告放 runner 的新临时目录；实际执行时登记源码版本、dirty、实际选择、命令、原请求/实例、结果、日志位置及清理事实。失败和补测各自保留。本机运行与 Linux 交叉编译分别记录，不将编译当运行通过。

## 未选择范围与人工验收

不默认运行 Full、完整 L2 live、全部 L3/L4 或 Runner 原生故障矩阵：本项改变调用方 owner/收尾接线，原请求与等待合并恢复由受控隔离链验证，原生执行规则未改。未操作 `C:\SteerlineInstall` 或 `C:\SteerlineRuntime`。后续确需临时调试先生成独立 `C:\SteerlineDebug` 包，AI 显式 `--simulate`。

CLI/页面和用户操作未新增，不单设人工前置或阻塞窗口；若实施发现必须改变用户可见行为，先同步本 Spec 的范围及验收要求。

## 实施与引用审计

分支 `codex/application-project-lifecycle`，基线 `ae6673e864437de32fd264b8842d7204f2694006` 加本次 working tree 修改；各报告的 environment 记录实际 source_commit、dirty、Go/OS/选择，E2E 另记构建 SHA。未借用第一批清理 PR 分支发布本项。

- 对 Go 生产/测试、平台文件、脚本与元数据检索后，删除 app 的 StartService/StartConfiguredService/RunConfiguredService、旧 window 启动、ServiceLifetime/Factory、serviceLocalAPI/serviceHTTPHost、startup 写入/握手等待与旧 stop_service 采样/回执/PID 等待。删去仅为旧 host 准入存在的 ApplicationAdmission 和 10 个退役测试权重；保留当前连接观察、旧格式拒绝、业务控制、锁/launcher 和精确进程能力。
- 新 `application_catalog.go` 仅持有配置正文/成员/读取诊断。目录显式 Start/Close，初始化无资源结果、排队 pending 和 runtime 完成结果分别产生；离线 verify-stop 直接读 catalog。BeginStop 的门槛在 cancel 前封闭；迟到 ready 检查目录 stopping，重复停止不把已结束行重新标成 stopping。
- `ProjectRuntime` 在 prepare 前建立；取得 repo/run-dir 锁、当前 writer、日志、Runner、product/handler 后统一收尾。新建项目直接把初始化 writer 交给 owner；已存在项目仍按原绑定检查打开 writer，移除 initializer 关闭后 factory 再开库的接线。底层打开失败仍由原存储构造函数回收尚未交出的连接，不扩展迁移或恢复。
- `newRunnerService` 只借用 store/CLI；ActorStore 不含 Close，Actor.EndActor 仅写 stopped checkpoint。原 Runner 停机确认/最终证明状态移至 `projectRunner`，保留 service-stop/startup-rejected 原 request_id；沿用原有独立预算的收尾补查机会，已确认 stop 仅补读失败证明，未知 stop 用原 ID。catalog 拒绝只清理本次新建实例；未通过接管/启动校验的实例只保留原观察，不擅停或写成功。
- 进程内 Read/Command/Task 共用 RWMutex 借用门槛，停止后新调用立即拒绝；已有调用退出后才关闭 handler/product/store/log。本地与 Runner 结果分别汇总；本地 close 失败保留锁与宿主的原策略不变。

结构守卫 `TestProjectLifecycleOwnershipBoundary` 防止旧声明回流、factory 关资源、Close 启动目录或 verify-stop 创建活动目录。具体行为覆盖不以结构检查代替。

## 自动验证证据

证据根为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`；以下目录均保留原始 summary 和对应日志。测试使用隔离 temp home/Git/SQLite/公开 CLI fixture，没有真实模型或安装现场。

| 覆盖 | 命令 / 结果 | 原始目录（相对证据根） |
| --- | --- | --- |
| ALC-A1 | `go build ./...` Windows 通过；临时 `GOOS=linux CGO_ENABLED=0 go build ./...` 通过，随后恢复环境；仅编译，不计 Linux 运行验收 | 命令执行回执及本记录 |
| ALC-A1 | `go run ./cmd/test-feedback -build-only`：52 包、complete/passed | `20261016T101259.549426500Z/summary.json` |
| ALC-A1–A5 | Plan 的 8 个 owner 包，另加改动的 `./cmd/test-feedback -count=1`：9 包，299 测试通过、0 失败、2 跳过 | `20261016T100420.175462800Z/summary.json` 与 go-test.jsonl |
| ALC-A1–A5 | `go run ./cmd/test-feedback -level=L1`：15 组、258 测试、0 跳过，complete/passed；补入未知启动用例后的最终复验单列下文 | `20261016T101009.324229900Z/summary.json`；首次同集合也通过：`20261016T100546.864808200Z` |
| ALC-A2–A4 | 最末 app 聚焦 `TestRunnerUnknownStart\|TestRunnerRejectedCatalog\|TestProjectRuntime\|TestUnifiedStartupLifecycleSharedPID\|TestUnifiedStartupLateReady\|TestUnifiedStartupCloseBeforeStart -count=1`：15 测试通过、0 失败/跳过 | `20261016T101120.090024100Z/summary.json` |
| ALC-A5 | `./scripts/test-deployment-stop.ps1`：正常停机、原失败独立核验、核验失败阻止部署全部通过；实际校验仍由 app 定向测试核对原配置/DB/候选/证明 | 命令回执 `Passed: staged stop, independent verification, and failed-verification deployment gate.` |
| ALC-A6 | `./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -Scenario 'runtime.service-lifecycle,recovery.waiting-merge-restart'`：selected-L3，2 场景 passed、cleanup_complete=true、无 cleanup_errors | `20261016T100607.170020600Z/summary.json` 和两个 `scenarios/<id>/scenario.json` |
| ALC-A6 | `./scripts/test-runtime-isolation.ps1 -Level L4 -Scenario runtime.service-write-isolation`：selected-L4，1 场景 passed、cleanup_complete=true、无 cleanup_errors | `20261016T101000.088864200Z/summary.json` 和 `scenarios/runtime.service-write-isolation/scenario.json` |

9 包定向的两个跳过是桌面 helper 子进程入口 `TestDesktopOpenHelper` 和需显式外部 Runner 的 `TestEnvironmentCheckPublishedContract`，不计通过，也不属于本项要求的 native 验收。已选择的固定 L1 和三项 E2E 不含跳过。

本次新增/迁移断言：本地 close 失败保留双锁、Runner 未证仍关 store/log、重复 close 不重放本地关闭；prepare 在已取得 writer/双锁后日志失败能收尾；实际 Read/Command/Task receipt 持有期间 close 必须等待且拒绝新借用；借用 product.Close 不关 writer/不影响兄弟；目录关闭前未启动、pending 不伪造成功和停止后迟到 ready；公开 CLI 的新建 catalog 拒绝收尾、旧实例拒绝不擅停、未知启动保留观察且文件未改；原停机未知补查与证明补读仍保持原规则。SQLite actor 测试保留控制文件原 digest 和最终 checkpoint/借用 store 断言，移除退役逐项目 stop receipt 测试。

初次 8 包定向因新增测试夹具把产品 Command 与存储同名 Command 混在一起导致 app 测试编译失败，保留失败报告 `20261016T100127.968787100Z`；拆分夹具后聚焦、9 包及 L1 均通过。首次新增资源聚焦 11 项也通过：`20261016T100219.162451800Z`。不将失败/编译或跳过计为产品场景通过。

## 最终复验与文档

最终 `go run ./cmd/test-feedback -level=L1`：15 组、259 测试通过、0 跳过、complete=true / passed=true；报告 `20261016T101327.802994400Z/summary.json`。架构、worker-lifecycle 当前实现差异、原停机验收测试位置、环境检查 Plan 的 owner 映射及 Windows Scenario 示例已同步；不改变 W/E/V 节点、异常和验证矩阵语义。

2026-10-16 最终静态检查：`python scripts/check_doc_links.py` 检查 456 个 Markdown、2732 个本地导航链接，0 错误；`git diff --check` 无诊断；本次改动/新增 Go 文件 `gofmt -l` 为空。

## 提 PR 前主干同步复验

合入 `origin/main@42621a79`（包含已合并的第一批清理、环境检查契约与 Runner 短状态路径），保留双方文档及当前单一资源 owner。原实现提交为 `472aec59`；以下报告记录该提交加合流 working tree，最终仅再更新设计/验收文字。

- 删除原 startRunner 时，将主干新增的 `runnerstatepath.Prepare/Directory` 一并迁入 `projectRunner.start`；只读 client/verify-stop 保留 Validate，不恢复旧启动 factory。日志取得前失败的目录断言跟随短目录；双项目夹具在已结算后清理各自短目录。
- Windows `go build ./...`、临时 Linux/CGO=0 生产编译通过；`go run ./cmd/test-feedback -build-only`：53 包 complete/passed，报告 `20261016T101850.441511100Z/summary.json`。
- `go run ./cmd/test-feedback -level=L1`：16 组、262 测试通过、0 跳过、complete/passed；报告 `20261016T101822.666426400Z/summary.json`，包含主干短路径 owner 组与本项全部固定用例。
- `./scripts/test-runtime-isolation.ps1 -Level L3 -Suite runtime -Scenario runtime.service-lifecycle`：selected-L3，passed=true、cleanup_complete=true；报告 `20261016T101835.140787200Z/summary.json` 与 `scenarios/runtime.service-lifecycle/scenario.json`。新建隔离现场，未覆盖前次证据。
- Plan 追加合流验证安排；架构图统一描述项目独立短目录。未重跑无关 Full/live 或完整 L3/L4，前述 recovery/L4 断言未修改。
- 最终文档链接 458 个 Markdown、2742 个本地导航链接，0 错误；diff 检查无诊断。原始证据根沿用本记录上文。
