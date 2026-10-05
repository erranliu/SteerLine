# Plan：执行适配与资源目录归组

任务 ID：78；任务名称：执行适配与资源目录归组。唯一执行依据：[Spec](three-line-06-adapters-resources.spec.md)。硬前置：#76。一个独立实现 PR 交付。

## 实施步骤

1. 从 PR219 合入基线盘点消费者。`codexapp` 的 adapter 与集成测试消费者共享 Codex app-server JSON-RPC/WebSocket 编解码，归 `internal/execution/adapters/protocol/codexapp`；`internal/steerline/remoteprotocol` 在当前基线只有四个 JSON 文件、没有 Go 源码或活动测试消费者，将 fixture 移入 remote adapter 的 testdata，不重建 verifier 或规划能力。

2. 将 Codex、Orca、本机 Orca remote implementation 分别移至 `internal/execution/adapters/{codex,orca,orcaremote}`；Git/PR 与 workspace 资源移至 `internal/execution/{publication,workspace}`。所有生产/测试消费者、Codex/Orca 配置 codec、原 binding 与 frozen envelope 的真实构造一并改线，删除旧路径。

3. 将 `internal/steerline/processhost.Host` 并入既有 `internal/execution/process`，复用 #76 已迁入的唯一 OS start/identity implementation；迁入原子文件替换及受控输出实现，删除重复的 processhost OS 实现。保留 `ManagedProcessHost` 方法、原子文件模式、Windows Job 和 Linux process group 生命周期、未知身份语义及 unsupported 平台拒绝。

4. 更新 app 的 `NewProcessHost`、`RunSessionLauncher`、`NewBuiltinDirectory`，以及配置、测试子进程、embed/tag、runner 路径与边界守卫。构造/API字段只按下表的单个修改所有者顺序更新，不扩展产品或后端行为。

5. 对照真实 Go 源码和跨 OS AST import 图修订当前架构文档、runner 与本 Plan。旧实现/资源目录均无生产或测试引用后删除；记录 #39 native evidence 复用或定向复测理由。

## 共享构造与路径所有者

| 文件/API或字段 | #78 修改责任 |
| --- | --- |
| `internal/steerlineapp/app.go`: `NewProcessHost(time.Duration) executionbackend.ManagedProcessHost`、`RunSessionLauncher(...)` | #78 唯一所有者；只绑定 execution/process 与 adapters/codex 的现有方法。 |
| `internal/steerlineapp/directory.go`: `NewBuiltinDirectory`、三种 `ExecutionRegistration`、profile codec keys 与 `Backend`/`Lifecycle` 工厂 | #78 唯一所有者；具体工厂指向迁后实现，冻结键、版本、注册字段与 admission 不变。 |
| `internal/steerlineapp/config.go`: process/workspace/publication 调用 | #78 唯一所有者；仅改 import/构造路径，不改 JSON 字段或配置接受规则。 |
| `internal/execution/application_config_types.go`: `Config.ProcessHost`、`Config.PR` | 类型所有者不变；只更新 `publication.Config` 的真实包路径，字段与序列化行为不变。 |
| `internal/coreboundary/check.go`、`internal/coreboundary/execution_boundary_test.go`、`internal/steerlineintegration/core_decoupling_67_boundary_test.go` | #78 唯一所有者；守卫从旧路径切到实际 adapters/resources，并增加禁用 usecase/task/storage/SQL/app/UI 的反例。 |
| `internal/testboundary/boundary_test.go`、`cmd/test-feedback/scoped.go` | #78 唯一所有者；生产 roots、E2E允许目录和 L1 精确 package/test 路径切换。 |

#77 的 product/api、product/query、client、console 和产品专属工厂仍由 #77 所有；本任务不修改它们的 DTO、查询或命令字段。若集成中 app factory 文件重叠，#78 只拥有上表列出的 adapter/resource 构造段。

实施中维护 `docs/validation/three-line-06-adapters-resources/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

下列入口直接使用基线中已有的代表 parent，加一个真实 app/resource wiring parent；旧 parent/subtest 与断言只改变包路径并记录在映射文件中。没有给 fixture-only `remoteprotocol` JSON 虚报活动测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/execution/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM06.1–TM06.2 Codex + app-server protocol | Windows；协议 fixture，无真实模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/adapters-codex/run-20261005-01 -- ./internal/execution/adapters/codex -run '^(TestCD63CodecIdentityAndVersion|TestAppServerLaunchPreservesConfiguredArgs|TestAppServerPendingFeedbackRecoveryUsesNewTurnReceipt)$' -count=1` | Codex adapter 与协议子包改线后 | 三个 parent 均实际 run/pass；保留 codec version、原 native args、原 session recovery request | `docs/validation/three-line-06-adapters-resources/adapters-codex/run-20261005-01/` |
| 定向 Go：TM06.1–TM06.2 本机 Orca | Windows；binding/terminal fixture，不接真实 Orca | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/adapters-orca/run-20261005-01 -- ./internal/execution/adapters/orca -run '^(TestOriginalTerminalRequiresAgentDetectionBeforeQualifiedProcess|TestFixedOrcaAgentTerminalRejectsReplacedIncarnation|TestAgentBindPendingReceiptDoesNotQualifyOriginalHost)$' -count=1` | Orca adapter 改线后 | 三个 parent 均实际 run/pass；保留原 terminal/runtime identity 与 detection gate | `docs/validation/three-line-06-adapters-resources/adapters-orca/run-20261005-01/` |
| 定向 Go：TM06.1–TM06.2 Codex app-server protocol | Windows；loopback WebSocket fixture | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/codexapp-protocol/run-20261005-01 -- ./internal/execution/adapters/protocol/codexapp -run '^(TestAskToolResponseUsesNativeAnswerEnvelope|TestCodexAppCallCancellationInterruptsBlockedWrite|TestCodexSocketHandshakeHonorsCancellationWithoutDeadline|TestCodexSocketLoopbackHandshakeAndMaskedJSON)$' -count=1` | 专用协议包迁入后 | 四个 parent 均实际 run/pass；唯一 codec 仍服务原调用方，取消/遮罩/握手断言保留 | `docs/validation/three-line-06-adapters-resources/codexapp-protocol/run-20261005-01/` |
| 定向 Go：TM06.1–TM06.2 remote refusal | Windows；存量 remote protocol/config fixture，不申请远端权限 | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/adapters-orcaremote/run-20261005-01 -- ./internal/execution/adapters/orcaremote -run '^(TestCD63CodecIdentityAndVersion|TestSupervisedOrcaPreflightPinsRemoteIdentityAndVersionFamily|TestSupervisedOrcaPreflightRejectsVersionOrCapabilityDrift|TestRemoteContinuationRequiresOriginalPeerAndTerminal|TestLoadConfigRequiresExplicitRemoteCancellationFalse)$' -count=1` | remote adapter 迁入后 | 五个 parent 均实际 run/pass；旧身份/version/refusal 保护通过，不产生 live admission | `docs/validation/three-line-06-adapters-resources/adapters-orcaremote/run-20261005-01/` |
| 定向 Go：TM06.1–TM06.3 资源与真实工厂 | Windows；隔离临时目录/真实本机子进程，无 Codex/Orca 会话 | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/wiring/run-20261005-01 -- ./internal/steerlineintegration -run '^TestTM06ResourceWiring$' -count=1` | app factory、process、资源路径迁移完成后 | parent 实际 run/pass；核对原子文件、受控输出、独立 generation 与三套 lifecycle 工厂的真实新包路径 | `docs/validation/three-line-06-adapters-resources/wiring/run-20261005-01/` |
| 定向 Go：TM06.1 resources | Windows；隔离 Git fixture | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/workspace/run-20261005-01 -- ./internal/execution/workspace -run '^(TestLocalWorkspacePrepareInspectResourcesAndCleanup|TestPrepareRejectsHeadChangedByCheckoutHook)$' -count=1` | workspace 消费者切换后 | 两个 parent 实际 run/pass；原 base/head/worktree 与清理约束通过 | `docs/validation/three-line-06-adapters-resources/workspace/run-20261005-01/` |
| 定向 Go：TM06.1 resources | Windows；command/provider fixture，无外网 | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/publication/run-20261005-01 -- ./internal/execution/publication -run '^(TestPublicationPolicyValidatesProviderValues|TestPRRequiresExactIdentityAndMergeEvidence|TestUnsupportedProviderDoesNoNetworkOperation)$' -count=1` | publication 消费者切换后 | 三个 parent 实际 run/pass；固定 PR 身份、merge evidence 与 unsupported refusal 通过 | `docs/validation/three-line-06-adapters-resources/publication/run-20261005-01/` |
| 定向 Go：TM06 Codex fixture path | Windows；已提交 native JSONL fixture，不启用原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/adapters-codex/path-repair-20261005-02 -- ./internal/execution/adapters/codex -run '^TestConversationMapperNativeTwoTurnFixture$' -count=1` | Codex package 移动后路径调整 | parent 实际 run/pass；实际读取原 JSONL 并保留 conversation turn/body 断言 | `docs/validation/three-line-06-adapters-resources/adapters-codex/path-repair-20261005-02/` |
| 定向 Go：TM06 Orca readiness fixture paths | Windows；已提交 readiness JSON fixtures，无原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/adapters-orca/readiness-path-20261005-02 -- ./internal/execution/adapters/orca -run '^(TestReadinessEvidenceUsesActualPlatformToolBlocks|TestReadinessReceiptRejectsStaleAndEscapingFiles|TestActualSavedCommandWrapperProvidesProbeEvidence|TestActualStringifiedCommandWrapperProvidesProbeEvidence)$' -count=1` | Orca package 移动后路径调整 | 四个 parents 实际 run/pass；读取原 JSON fixtures 和 wrapper evidence | `docs/validation/three-line-06-adapters-resources/adapters-orca/readiness-path-20261005-02/` |
| 定向 Go：TM06 Process Host caller compile/use | Windows；隔离 project fixture，无原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/process-api/run-20261005-02 -- ./internal/steerline -run '^TestNativeProjectInitializesOnceAndRejectsMismatchedBinding$' -count=1` | `StartManaged` 具体返回值保留 Windows process-tree 检查后 | parent 实际 run/pass，Windows caller 编译期仍可见原 `ContainsProcess` 能力 | `docs/validation/three-line-06-adapters-resources/process-api/run-20261005-02/` |
| 定向 Go：TM06.1 adapter boundary | Windows；临时 source fixtures，涵盖 inactive build tag、transitive helper、testkit 与 direct product/taskview imports | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/boundary/run-20261005-03 -- ./internal/coreboundary -run '^(TestTM04ExecutionBoundary|TestTM06AdapterBoundary)$' -count=1` | adapter forbidden/import exception 守卫最终修复后 | 两个 parent 及全部禁止/允许子例实际 run/pass；production adapter 拒绝 testkit，Windows/Linux graph 通过 | `docs/validation/three-line-06-adapters-resources/boundary/run-20261005-03/` |
| 定向 Go：TM06.1 实仓 import closure | Windows；扫描当前源码与 Windows/Linux production graph，不触发 native opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/integration-boundary/run-20261005-02 -- ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1` | #78 所有者/纯值 import 例外稳定后 | parent 实际 run/pass；全 OS/tag AST 与 Windows/Linux 传递依赖图无 violation | `docs/validation/three-line-06-adapters-resources/integration-boundary/run-20261005-02/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-06-adapters-resources/l1/run-20261005-03` | 所有定向修复与自查完成后一次最终验收 | 适配/资源、app wiring、规则与 all-tag boundary parent 非零命中并通过，无 skip/live | `docs/validation/three-line-06-adapters-resources/l1/run-20261005-03/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

未选择 Full/L2/L3/live：启动参数、权限、协议及资源能力无行为变更，复用 [#39 本机原生 wiring](../validation/candidate-native-acceptance/native-wiring-final/summary.json)、[候选现场](../validation/candidate-native-acceptance/local-08/summary.json) 与 [原 L1](../validation/candidate-native-acceptance/l1-final/summary.json)，本任务另跑一次当前分支 L1。若自查发现真实行为变化，先把具体受支持平台的原生命令与原身份回执证据补入 Plan 再执行；remote live 流程不属于一期，存量 remote 只做迁移/编译/协议或拒绝代表回归，不要求真实远端准入。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-06-adapters-resources/README.md`，注明复用证据、定向重验依据及未具备能力。

#80 接新工厂与资源小端口；#39 记录固定构建，迁移后只重验 native-impact.md 中发生实际变化的入口。
