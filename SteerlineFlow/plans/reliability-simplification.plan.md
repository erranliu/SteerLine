# 可靠性交付简化实施 Plan

依据[经二轮审计的方案](../../docs/architecture/reliable-delivery-chain.md#可以简化的流程)，按其依赖实施，不修改 roadmap。Worker 规则沿用 [W/E/V 矩阵](../../docs/product-design/worker-lifecycle.md)，不新增输入重放或资源停止授权，不使用安装环境作测试。

## 实施顺序

1. 判定与表达：Go 在同一次启动协调中完成停止、按更新目标核验、必要且已授权时恢复原安装一次；输出绑定本次 home、目标及 Runner 摘要的结构化结果。PowerShell 保留调用、退出码、包绑定、文件/进程边界检查及证据落盘，不复制 Runner/关闭的领域判定。reset 及只读取证不进入该可变入口。清理相反的历史长期规则。
2. 实际受理前绑定：Runner 公开完整 binding、逐命令能力声明与停止/受理顺序；调用方接通后才减少已覆盖预查询。实际 EXE 校验保持。
3. 启动证据与安装排他：原 census 连续性、公开共享/独占租约、namespace 归属、owned 启动上下文及 pending 启动阻断；缺前提不启用离线简化。
4. 离线替换与续装：同进程持锁、不可变 prepared、实际旧/目标字节判定、原子 pending/manifest 提交及 fresh writer 停止优先。
5. 普通产品命令收口：仅承载位置收口，保留已有效果补查、领域事务、候选 outbox 与重启零补发。
6. 同快照观察：Runner 公开同事务投影/事件/cursor，调用方 CAS 整页保存；交付前 PR 刷新与 EXE 散列保留。

新协议不假设已存在；不增加历史数据迁移或双写。各阶段只有对应公开能力与测试均通过才启用，不以设计报告充当运行证明。实时结果统一登记于[专属验收](../testing/acceptance/reliability-simplification.md)。

## 测试计划

各项映射方案的 SIM-A01–A09，结果使用专属验收 ID。下列命令在源码工作区执行、临时目录夹具隔离；每一批完成实现后执行其相关命令，失败保留原日志，不自动重试或转安装现场。

| 验收项 | 级别 / 场景与命令 | 环境、时点和通过条件 | 证据位置 |
| --- | --- | --- | --- |
| RS-A01（SIM-A01） | L1，`go test ./internal/app ./cmd/steerline -run 'TestDeploymentPreparation\|TestDeploymentRecoveryArguments\|TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite\|TestDeploymentRestartCandidateBindingsPreserveOriginalState' -count=1 -timeout=120s`；`pwsh -NoProfile -File scripts/test-deployment-stop.ps1` | Go 入口与脚本接线后；目标不能越权、失败阻断、只恢复一次、当前失败回执不可绕过、保留原未知资源事实、错 home/目标/摘要不继续安装；原关闭与接管基线保持 | `.evidence/reliability-simplification/phase-a/` |
| RS-A02（SIM-A02） | L1，Runner：`go test ./internal/host ./internal/cli -run 'TestSRFullBinding\|TestSRExpectedInstance\|TestSRShutdownProofRequiresReleasedWriter' -count=1 -timeout=90s`；SteerLine：`go test ./internal/runnerclient -run 'TestRunnerFullBindingAdmission\|TestRunnerPresetFreeze\|TestSteerRunnerReleasePinBlocksBeforeInvocation\|TestServiceSettlementRequiresOriginalFinalProof' -count=1 -timeout=60s` | 受理端先实现，再接调用方，最后删预查询；错误实例/构建/资源/目录/stopping/缺能力零副作用，原请求不改写 | `.evidence/reliability-simplification/phase-b/` |
| RS-A03（SIM-A03–A05） | L1 多进程临时目录 fixture；实施租约、清单和 owned 上下文时落实场景入口 | 启用离线简化前；真 OS 共享/独占锁、清单缺失、重启不得空库掩盖、新 writer 插入和原 stop 优先全部成立 | `.evidence/reliability-simplification/phase-c/` |
| RS-A04（SIM-A07–A08） | L1 临时 home 的每步中断/续装夹具；实施前登记入口 | 启用续装前；pending 崩溃阻断、旧/目标/第三种字节、配置冻结、原子 manifest、NoStart/启动失败分别记录 | `.evidence/reliability-simplification/phase-d/` |
| RS-A05（SIM-A06） | L1 actor/SQLite 与受控 Runner fixture；实施命令承载时登记入口 | 移除文件通道前；原效果可查、HTTP 丢响应、旧输入零重发、同 ID 改正文拒绝、unavailable 不变 not_found | `.evidence/reliability-simplification/phase-e/` |
| RS-A06（SIM-A09） | L1，Runner：`go test ./internal/storage -run 'TestSRObservation\|TestSRRunAtomicCommit' -count=1 -timeout=90s`；SteerLine：`go test ./internal/runnerbridge ./internal/runnerclient -run 'TestRunnerObservation\|TestSteerRunnerOutboxBoundaries\|TestSteerRunnerRestartOriginalBinding' -count=1 -timeout=90s` | 替换观察分支前；并发/分页/空页后新事件不丢失、不超 revision，CAS 不保存半页 | `.evidence/reliability-simplification/phase-f/` |

尚未有实际命令的后续阶段不宣称自动验收可执行：相应能力、场景与命令必须随该阶段实施补齐，验收之前落实；不删除现有门槛。内部判定收口已有 L1 可覆盖，不默认选择 L2/L3/L4、全量或真实模型；跨组件新增能力须使用实际公开 CLI/真实 OS 租约夹具，不能仅模拟 API 返回。

真实公开 CLI 联验：先将 Runner 构建到本批证据目录，设置 `STEERRUNNER_EXE` 为该绝对路径、`STEERRUNNER_VERSION` 为其 version 输出、`STEERRUNNER_SHA256` 为实际散列，再运行 `go test ./internal/runnerclient -run '^TestRunnerFullBindingPublicCLI$' -count=1 -v -timeout=90s`；必须有测试终态且无 skip，正式关闭本次空闲服务并核实结算。不运行原生模型。部署两语言接线在临时空 home 中用生产脚本函数调用同目录的实际暂存 CLI/Runner 与只供散列的 Web 文件，核对调用 ID、整包摘要和原资源不提升；实际安装不在该验证范围。

RS-A01 较慢的旧恢复夹具独立运行：`go test ./internal/app -run '^TestDeploymentInterruptedRecovery$/^(stale|missing|preparation-update|preparation-failed-cycle|preparation-replace)$' -count=1 -v -timeout=300s`。其余判定/原关闭/CLI 用前述小范围命令，避免将多个旧夹具累加到一份 120s 总预算；正式固定 L1 分区本身的预算保持。

本批收尾：用 Windows PowerShell 5.1 再执行同一脚本夹具；`go test ./internal/testkit/boundary -run 'TestCallerOwnerBoundary|TestTakeoverContractOwnership|TestTM04ExecutionBoundary|TestTM06ContractValueClosure' -count=1 -timeout=90s` 验证公开边界，证据 `boundary.log`。执行文档链接检查及两仓 diff 格式检查，不以此扩大业务验收。
