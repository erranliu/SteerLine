# #68 主包解耦重构后代码清理

依据 [Spec](../../spec/core-decoupling-07-cleanup.spec.md)、[Plan](../../spec/core-decoupling-07-cleanup.plan.md)及两个共同约束。实际工作区 `C:/Users/lmas1/.codex/worktrees/fc3b/SteerLine`；起点为最新 main `d8bacbd75994a183c6d54c027abe103d26ae6896`，PR190 平台确认 `mergedAt=2026-10-03T05:16:14Z`、最终 head `f77724a066afd0c43b7fda28c9bbabf77954a9b8`。分支 `codex/68-core-cleanup`，执行设置 `gpt-6.1-sol / high`，覆盖 runner 默认设置。本任务只交付清理 PR，等待协调独立复审/修复与真实合并；未启动 #69–#72。

## 删除依据与唯一所有者

[逐项候选清单](candidate-inventory.json)记录 21 个私有 helper 的原职责、唯一所有者、保留调用链、各消费者类别、构建条件、原函数摘要和历史提及。全 tracked Go（含测试、全部 OS/tag）每个符号仅出现一次，均为自身声明；脚本、产品 CLI、协议 fixture、嵌入资源和反射没有动态消费者。这个扫描只是证据之一，删除另以实际最终调用关系审阅为依据。

| 删除项 | 原职责 / 最终唯一所有者及实际消费者 |
| --- | --- |
| `nativeDependencyState`、`nativeDeliveryInconsistentStatus`、`nativeStartupBlockersFor`、`nativeStartupBlockers` | 任务资格/交付/启动阻断旧包装；`task/readiness.go` 唯一规则，sqlite/tasks.go、taskview/project.go 和中心 Claim 继续消费。前三项所在 `native_readiness.go` 删除；`native_readiness_query.go` 保留实际使用的 delivery/dependency 包装及公共 DTO 别名。 |
| `latestWorkflowExecution`、`latestCurrentNativeAttempt`、`latestNativeExecutionAttempt` | 旧 latest/current 选择；实际投影在 `projection/selectors.go`，`runtime.go`/`run.go` 直接消费。恢复按原 `CurrentAttemptID`，未把旧时间选择规则接回生产。 |
| `implementationEnvelope` | 旧按 key 读取桥；中心 `validateExecutionImplementationData` 与 固定目录身份/verifyPolicyResources 校验身份/重复，实现 codec 解释载荷。put/remove/awaitingRecoveryResources/SessionFacts 全部保留。没有前置临时原生 decoder 缺口。 |
| `clonePublicationSettingsValue` | 未使用的五类 publication slice 复制副本；`ExecutionProfile.Freeze` 内原五类 slices.Clone 全部保留，不改冻结或复制行为。 |
| `hostFact`、`hostActionFailure` | 未用宿主值/错误包装；实现通过 HostEnvironmentPort 返回 HostFact，中心 `readHostOutput`/`blockWorkerEnvironment`/`executeHostAction` 继续实际消费并提交。 |
| `appServerAskDecision` | 未用问题包装；`worker_events.go` 实际调用 `decisionRequestFromEventFact` / `WriteDecisionRequest`，原决策与路由校验保留。 |
| `deliverDurableWorkerEvent` | 未用的 Events/Persisted 等待辅助；实际 observer → workerMessages → commit 后授权 ack 链不变。`durableWorkerProcess` 仍是中性提交失败测试的 ProcessHandle，保留。 |
| `enableTestLifecycleMessages`、`stopTestLifecycleMessages` | 无消费者的私有通道 fixture；整份 `lifecycle_messages_fixture_test.go` 删除，内含零测试/零断言。外层 `lifecycle_test.go` 经真实 app/Service 与 cleanup 验证，不使用这组 helper。 |
| `queryNativeRequirementTool` | 无消费者的测试 unknown 转发；中性目录仍消费 queryHost/queryRemote，app 持有原生查询。没有删除无认证拒绝。 |
| `sanitizeName` | app 配置中未用名字规范化副本；产品状态路径继续由 `repositoryStateKey` → `steerline.RepositoryStateKey` 生成，shortHash/配置入口不变。 |
| `remoteWorkerError` | 组装中未用错误转换副本；`orca_remote_worker/backend.go:backendError` 由实际 Backend 方法消费，仍转换原错误/原因。 |
| `fixtureOriginalResource` | 无调用的直接造 Execution 绑定 fixture；内部唯一 `require(t, err)` 是无人执行的封套构造检查，没有测试覆盖依赖它；外层 `newLifecycleProject`/builtinFixtureConfig 及真实 receipt 提交链保留。 |
| `settledEvidence` | 未用转发方法；`lifecycle_observation.go:Observe` 直接调用 `nativeTurnEvidence`，原 TaskDelivery/结算事实不变。 |
| `parseTerminalFromSaved` | 未用 receipt 解析副本；createAgentTerminal 用 `ParseFixedAgentTerminalReceipt`，reconcileAgentTerminal 用 `savedNativePayload` + `FixedAgentTerminal` + 原终端 list 与 process/incarnation 校验。没有删除未知结果对账或原身份拒绝。 |

本次没有发现需改接的第二条活跃规则实现、活跃重复 fixture 或空分支。只删除无人消费的副本，不按名字相似、失败路径或数量目标清理。其余非本次解耦链上的未用显示/控制辅助没有纳入此次清理。

## 消费者与构建条件复核

全量扫描覆盖删除前 543 个 tracked Go 文件，包括 Windows/Linux/其他 OS 后缀、`e2e` 等 tag 和 `_test.go`；21 个函数所在文件均无 build tag/OS 后缀。具体脚本/嵌入/调用入口：

- `cmd/steerline/main.go` 通过 app LoadConfig/NewService/RunConfiguredService/目录接线；console 显式注入来源。
- `cmd/test-feedback/scoped.go` 固定 quick 的 15 包、parent/subtest 清单及嵌入权重 JSON，删改符号均未被这些选择引用。
- `cmd/orca-probe`、`scripts/`、`tests/e2e` 的 driver/native/evidence、`examples/` 与 mock 协议输入没有私有符号调用或字符串选择。
- `launchers.go` 的 `launcher/*` 及 runner 权重是仅有相关嵌入入口，未消费被删 helper。没有 `go:linkname` 或 MethodByName 分派；reflect 用于值等价核验。被删方法不满足任何接口方法声明，也不是协议 handler/注册回调。

源码/index 的最终文件计数与全 OS/tag imports 见 [source-index.json](source-index.json)；#67 quick 守卫负责 AST 完整图、传递/反向边及 Windows/Linux 生产依赖。额外完成 Windows/Linux 所有产品入口构建。Linux 交叉编译不声明执行过 Linux 测试或新增平台支持。

保留公共/协议职责：`domain.go` 的业务 DTO 别名和 `NativeReadyTaskSnapshot` 仍供 command/query/console 消费；`WorkerProcessEnvironment` 是 app 的子进程秘密隔离入口；app `NewServiceWithLifecycleResolver` 是外层故障包装公开构造；生命周期 wrapper 满足正式端口，不能因为内部只有转发就删除。旧格式拒绝、原生 opt-in/历史证据、remote 无权限 Claim 拒绝全部保留。没有新增版本、迁移、双写、回退、新平台、权限或写端口。

## 断言归属与最小验证

无测试函数或断言删除/迁移，旧到新映射为原路径 → 原路径；删除的三组四个测试 helper 均无消费者；其中 fixtureOriginalResource 的唯一 `require(t, err)` 无执行入口，封套构造/身份拒绝仍由现存 codec 与真实 receipt 链验证，其余三项不含断言。#63–#67 的原 [断言映射](../core-decoupling-06-boundary/test-mapping.json) 和 [fixture 映射](../core-decoupling-06-boundary/fixture-mapping.json) 保留为当时的历史清单，当前 21 项删除由本记录补充，不回写历史。

| 受影响职责 | 保留断言与实际选择 |
| --- | --- |
| intent、receipt、提交失败/observer 清理、固定 profile 复制 | 中心 4 个定向 parent：WorkerReceiptFailure、WorkerIntentIsDurable、WorkerCommitRollsBack、WorkerFlowContextsDoNotExchangeExecutionProfiles；详见 Plan 精确命令 |
| 任务资格/交付、投影 current 选择 | quick core Claim/provider delivery，query/taskview/projection 既有包用例，维持原断言 |
| 封套/配置/原生身份、未知结果/准备门槛/唯一 TaskDelivery | quick execution/contracts、owner codec/identity/remote/app、CD67 AdditionalImplementation 原固定输入、恢复和 receipt/ack 屏障 |
| 全标签隔离与产品接线 | quick `TestCD67CoreBoundary` 及全部七个负向样本，第三实现与支持/remote 子例；不重复独立矩阵 |

首轮 `bin/cd68/focused` 编译失败：`worker_lifecycle_async.go` 遗留未用 execution import，0 tests；修正后用新报告目录重跑，原失败保留。最终结果、代码候选 SHA、报告摘要/JSON 命中、构建和 diff 结果归 [validation-results.json](validation-results.json)、[test-hits.json](test-hits.json)。

最终代码候选 `55357397f7dac9997a75006760fe991f28f510ff` 的一次既有 quick 在干净工作树上 **complete/pass**：15 包各一次、144 个 parent/subtest 均有唯一 run/pass、0 fail/skip、errors=[]，执行 226.0s。修正后定向 **9/9 parent/subtest pass**（4 个顶层用例），50.2s。逐组命令、实际包/用例、stdout/stderr SHA-256 均已归档，没有用总数推断命中。

守卫实测：直属主包 172 个 Go 文件、完整源码 34 包、Windows/Linux 生产依赖 232/221，0 违规；七个额外 OS/tag/目录/反向依赖负向样本均真实命中并拒绝。Windows/Linux amd64 的 `cmd/steerline`、`cmd/test-feedback`、`cmd/orca-probe` 构建 exit 0；标准 `git diff --check` 与提交 diff check 通过。初始架构文档 EOF 空行已修正；一次关闭 autocrlf 的诊断把 CR 行尾当作 trailing whitespace，未改仓库换行规则，最终按仓库标准规范化 diff 检查通过。后续仅文档归档，Go 源码未变；未声明协调独立复审或已合并。

所有测试显式使用临时 STEERLINE_HOME、隔离 Git/SQLite/协议 mock，无 opt-in/live 开关；没有调用真实用户产品 CLI/业务服务/任务库或 `C:/SteerlineRuntime`，没有恢复或覆盖历史 execution.json、修改 runner/仓库外协调状态。未跑 full/L2/L3/live；本次删除不改变原生启动、权限映射或支持范围。
