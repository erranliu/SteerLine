# 调用方边界收口实施 Plan

2026-10-20，用户直接授权当前聊天实施，基线 `0752537e`。只处理架构审查前三项，不进入 roadmap。

## 实施

1. HTTP 消费端只允许独立的 `processwindow` 基础设施包；为该包及 runnerclient、runnerbridge、environmentcheck、调用方纯绑定规则补直接/传递依赖与未激活标签反例。
2. runnerbridge 唯一生成并保存候选 stop 信封；正常对账与启动前准备共用入口，保留两条路径既有 ID 生成方式、CAS、未知原信封及明确拒绝后的再准备。app 继续取得项目锁/store，启动前持久化并提交 pending_stops。
3. takeover 公开类型与协议校验归 runnercontract；原 Candidate/Check 绑定及构建比较归纯 runnerbinding，启动、离线重启和查询复用。部署在线预检调用暂存 SteerLine 的同一 Go 入口，失败仍在停止/替换之前阻断。

节点/异常/验证以 [worker-lifecycle](../product-design/worker-lifecycle.md) W02–W18/E04–E07/V03、V06–V08 为准。本轮只调整规则归属；原输入授权、零自动 submit/reply/resume、只读接管限制、原 Run/input/request/build 事实、停服与资源结算分别取证以及唯一交付事务保持。未知不生成新信封；不增加兼容旧数据或恢复入口。

## 测试计划

验收归[专属文档](../acceptance/architecture-boundary-consolidation.md)，下列均为本任务自动项，无人工项。

| 验收项 | 级别/组与场景 | 环境、命令和时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| ABC-A01 | L1 quick-execution-boundary，全部既有和新增所有者守卫 | 完成实现后 `go run ./cmd/test-feedback -out <临时证据>/boundary -- ./internal/testkit/boundary -count=1`；隔离源码夹具和 Windows/Linux 生产依赖图 | 正例通过，直接/间接/未激活源码反例被拒绝；同目录 summary/jsonl |
| ABC-A02 | L1 quick-owner-runner-bridge，原 outbox 与新增启动 stop 准备场景 | `go run ./cmd/test-feedback -out <临时证据>/core -- ./internal/runnerbridge ./internal/runnerclient ./internal/runnerbinding ./internal/runnercontract -count=1`；内存 Store 和公开 CLI fixture | 保存失败零发送，未知/重复准备字节不变，明确拒绝可按原规则准备，停服和公开协议回归；同目录报告 |
| ABC-A03 | L1 quick-owner-unified-startup、环境检查、部署定向集成 | `go run ./cmd/test-feedback -out <临时证据>/app -- ./internal/app -run 'TestSharedRunner|TestTakeover|TestStartupCancellation|TestDeployment|TestEnvironmentCheck|TestRunnerShutdown|TestProjectRuntime' -count=1`；原 Git/SQLite/CLI 隔离夹具 | 当前构建/原事实/检查/原停止绑定保持，启动与离线规则一致；同目录报告 |
| ABC-A03 | 部署脚本局部验证与消费者编译 | `./scripts/test-deployment-stop.ps1`，`go run ./cmd/test-feedback -out <临时证据>/cli-selection -- ./cmd/steerline ./cmd/test-feedback -run 'TestRunnerTakeoverPreflight|Test.*Application|TestQuick|Test.*Owner|Test.*Boundary' -count=1`，PowerShell Parser 与 `git diff --check` | 在线预检委托同一 Go 入口；失败零停旧服务/零替换，原结算事实保持；输出登记到验收 |

若收口协议校验时涉及调用方比较条件，补跑 `go run ./cmd/test-feedback -out <临时证据>/identity-final -- ./internal/runnerbinding ./internal/app -run 'TestTakeover|TestSharedRunnerCompatibleBuildUsesPublicPreflight|TestStartupCancellationOutboxIsDurableBeforeRunnerStart' -count=1`，覆盖 ABC-A02/A03；保留调用方 SHA 精确比较及按宿主 OS 的路径比较，不将 wire 层的格式接纳等同于原事实匹配。

提交前自查沿上述 ABC-A01–A03 复核生产差异，并定向重跑 core、`TestTM04ExecutionBoundary|TestCallerOwnerBoundary|TestTakeoverContractOwnership` 和部署脚本。路径身份用例同时覆盖 Unicode 大小写折叠与原清理后转小写算法的差异。若公用测试夹具需改动，补跑 `go run ./cmd/test-feedback -out <复核证据>/app-fixture-final -- ./internal/app -run 'TestStartupCancellationOutboxIsDurableBeforeRunnerStart|TestTakeover|TestSharedRunnerCompatibleBuildUsesPublicPreflight|TestDeploymentRestartCandidateBindingsPreserveOriginalState|TestDeploymentCandidateVerificationKeepsOriginalBinding|TestRunnerPresetFreeze' -count=1`，覆盖 ABC-A02/A03 及其全部公用夹具消费者；只让原 actor 观察公开事实，不新增独立 writer、生产重试或放宽 CAS。

夹具准备的控制回执使用已冻结宿主的既有 `WaitControlReceipt` 校验入口，保留原 8 秒期限，不逐次重建配置/Git 身份。修正后定向补测同一命令中的公用夹具消费者 `TestRunnerPresetFreeze/(reject_mismatched_run_after_explicit_restart|accept_replacement_after_explicit_restart)$` 及原启动/绑定/部署项，证据另存 `app-fixture-verified`；不兼容接管反例通过 `-run 'TestRunnerPresetFreeze/reject_changed_build_without_compatible_takeover$'` 单独执行并保存 `incompatible-takeover-final`，核验拒绝、原 pin 不变、原服务保持退出（ABC-A03）。

新增规则/场景同步固定 L1 选择登记。此次为内部规则收口，已有局部集成可覆盖变化，不选择 L2/L3/L4、Full、真实模型或安装现场；不修改安装及保留运行环境。Plan 不维护运行结果。
