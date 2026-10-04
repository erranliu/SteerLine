# Plan：任务选择与执行结果边界

任务 ID：74；任务名称：任务选择与执行结果边界。唯一执行依据：[Spec](three-line-02-task-execution-boundary.spec.md)。硬前置：#73。一个独立实现 PR 交付。

## 实施步骤

1. 定义并实际注入任务工作与结果端口，将任务纯规则中的执行校验移动到唯一执行侧；同步 queries 的资格原因。

2. 沿首次领取/后续候选路径改调用，保持同一原子提交；迁 PR 集合核验及任务级结果应用，保留原请求/版本。

3. 迁丢弃执行核验/清理责任；拆共享 Service/CommandService/Config 协调段，交付 integration-ownership.md 标注字段、方法与后继文件所有者。

实施中维护 `docs/validation/three-line-02-task-execution-boundary/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM02.1 | Windows；隔离 SQLite、可控后端 | `go run ./cmd/test-feedback -out docs/validation/three-line-02-task-execution-boundary/claim/run-03 -- ./internal/steerline -run '^(TestMC1\|TestMC2\|TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation\|TestNativeClaimAllowsManualParallelButKeepsAutoSlot\|TestTM02TaskWorkPort)$' -count=1` | 真实启动切换后 | 保留首次/后续身份与领取/模式代表断言，全部非零命中 | `docs/validation/three-line-02-task-execution-boundary/claim/` |
| 定向 Go：TM02.2–TM02.3 | Windows；隔离 SQLite/provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-02-task-execution-boundary/result/run-03 -- ./internal/steerline -run '^(TestMC5\|TestMC6\|TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor\|TestTaskDiscardRequestPersistsUnknownAndThenDiscardsOnlyAfterUnmergedProof\|TestTM02ResultCAS)$' -count=1` | 结果和丢弃同事务接通后 | 已有交付/冲突代表通过；补结果去重、旧版本及交付/丢弃竞争缺口 | `docs/validation/three-line-02-task-execution-boundary/result/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-02-task-execution-boundary/l1/run-03` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-02-task-execution-boundary/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：身份/事务责任变化由受控实际生产接线验；不重跑原生权限矩阵。L3 组合留 #82；若新发现恢复路径受影响则只追加 recovery.waiting-merge-restart。

实施后的精确命令与父/子断言清单见 [test-mapping.json](../validation/three-line-02-task-execution-boundary/test-mapping.json)。首次领取原预留重放的版本前值语义保持原断言；规范结果的旧任务/图/集合版本及异内容重放拒绝分别由 TestTM02ResultCAS 验证。task 原测试按断言所有者迁到 `internal/task/work_test.go` 与 `internal/execution/task_boundary_test.go`，PR 拒绝与任务声明分开；原 merged-discard 断言迁入真实 SQLite/生产执行协调的 `TestTM02ResultCAS/merged_discard_rejected`，没有削弱该检查。

补充最小测试计划（实际构造与 schema 14 涉及重启重新注入及新结果持久关联，只加既有 waiting-merge controlled 场景）：

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向规则/格式/原生小端口代表 | Windows；隔离 Git/SQLite/provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-02-task-execution-boundary/extra/run-02 -- ./internal/task ./internal/execution ./internal/steerline/sqlite ./internal/steerline -run '^(TestTaskStatusAndDeliveryEvidence\|TestDeliveryGateRequiresSelectedWorker\|TestBatchPlanAndTaskClaimKeepFixedValues\|TestNativeDatabaseInitializesCurrentSchema\|TestTM02PreviousResultSchemaRejected\|TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged\|TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent\|TestTaskDiscardDuringActiveExecutionKeepsSlotAndCleanupIdentity\|TestNativeClaimPersistsChosenExecutionProfile\|TestWorkerFlowClaimsFreezeIndependentLocalProfiles)$' -count=1` | 共享拆分与构造完成后 | 原迁入断言、旧 schema 拒绝、选中 profile/PR 和丢弃精确证明实际非零命中，无 skip | `docs/validation/three-line-02-task-execution-boundary/extra/run-02/` |
| L3 controlled：recovery.waiting-merge-restart | Windows；独立临时项目，真实 CLI/Git/SQLite 与受控 provider/helper | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario=recovery.waiting-merge-restart -out docs/validation/three-line-02-task-execution-boundary/recovery/run-01` | 新提交/构造稳定后 | 重启保留原 Task/Workflow/Execution/workspace；不重派，正常交付且清理完成，0 fail/skip | `docs/validation/three-line-02-task-execution-boundary/recovery/run-01/` |
| Windows/Linux 生产编译与图 | 独立 GOOS 子进程，不冒充 Linux 执行行为 | `go build ./...`（Windows）；`$env:GOOS='linux'; go build ./...`；L1 的 coreboundary 直接/传递图 | L1 后 | 两个生产目标编译成功，所有 tag/OS 源码守卫及依赖图通过 | `docs/validation/three-line-02-task-execution-boundary/build/`、`l1/run-03/` |

测试入口每次使用新空子目录。首次失败报告保持原文，修复后按受影响入口重跑，不能将旧失败或零匹配覆盖成通过。不选 Full/L2/live，也不运行整个 L3；没有新增原生权限、协议、远端准入或模型请求。

L1 首次运行确认 task 包无测试产生 package skip，且旧 CM69 仍指向已删除的 Snapshot owner。修复仅迁入原任务断言并更新实际纯投影所有者；taskview 新增全 OS/tag 禁止依赖负向覆盖。精确修复命令为 `go run ./cmd/test-feedback -out docs/validation/three-line-02-task-execution-boundary/l1-repair/run-01 -- ./internal/task ./internal/execution ./internal/steerline/persistence ./internal/steerlineintegration -run '^(TestTaskStatusAndDeliveryEvidence\|TestDeliveryGateRequiresSelectedWorker\|TestBatchPlanAndTaskClaimKeepFixedValues\|TestCM69PurePortDependencies\|TestCD67CoreBoundary)$' -count=1`，随后重跑同一 L1 固定入口，保留失败报告，不放宽 skip/零命中/禁止依赖判定。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-02-task-execution-boundary/README.md`，注明复用证据、定向重验依据及未具备能力。

#75/#76/#77 只能消费本次已接通的窄端口；清单指定共同文件集成所有者，不把接口草案当交付。与 #39 并行时固定其构建，变更交付路径只定向复核受影响证据。

## 协调复审与最新 main 集成测试计划

集成 main 的 PR213 候选交互更新。复审将工作纯值归到 task/model.FixedWork，并将 TaskWorkPort/TaskResultPort 定义放在当前执行消费方；任务实现由 app 注入并通过结构匹配满足端口，执行生产代码不再 import task 实现。只调整定义归属与构造签名，不改变业务状态/提交语义。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| L1 固定最小入口 | Windows，隔离 STEERLINE_HOME，无 opt-in | `go run ./cmd/test-feedback -level=L1 -out C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-three-line-20261004-01a10589/review74-l1-main-integrated` | 集成 main 后、消费方端口整理前 | 原构造/规则/全 OS 源码与 Windows/Linux 依赖守卫通过 | 协调 run 的 review74-l1-main-integrated/ |
| 定向 Go：任务工作/结果 CAS/重复回执/交付丢弃竞争 | Windows，真实 SQLite/受控 provider，无模型 | `go run ./cmd/test-feedback -out C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-three-line-20261004-01a10589/review74-boundary-final -- ./internal/task ./internal/steerline ./internal/steerlineintegration -run '^(TestTM02TaskWorkPort\|TestTM02ResultCAS\|TestTaskStatusAndDeliveryEvidence\|TestBatchPlanAndTaskClaimKeepFixedValues\|TestCandidateConsoleWiring)$' -count=1` | 纯工作值迁移后 | 非零 pass，无 fail/skip，原身份、集合版本、去重和 CAS 保持 | 协调 run 的 review74-boundary-final/ |
| 定向 Go：消费方端口/实际应用/禁止依赖 | 同上，无原生权限开关 | `go run ./cmd/test-feedback -out C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-three-line-20261004-01a10589/review74-port-wiring -- ./internal/task ./internal/steerline ./internal/steerlineintegration -run '^(TestTaskStatusAndDeliveryEvidence\|TestBatchPlanAndTaskClaimKeepFixedValues\|TestTM02TaskWorkPort\|TestCandidateConsoleWiring\|TestCD67CoreBoundary)$' -count=1` | 端口定义和全部构造签名切换后 | 非零 pass，无 fail/skip；真实注入及跨平台源码/依赖矩阵正确 | 协调 run 的 review74-port-wiring/ |
| 定向 Go：当前 main 控制台消费者 | Windows 产品 fixture，无模型 | `go run ./cmd/test-feedback -out C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-three-line-20261004-01a10589/review74-console-main -- ./internal/steerline/console -count=1` | main 组合接通后 | 最新候选交互与 UI 身份/输入/展示代表通过 | 协调 run 的 review74-console-main/ |

复用本任务既有 controlled recovery.waiting-merge-restart 与 schema 14 拒绝证据；复审不改事务或恢复行为，不重复 Full/L2/L3/live。最后消费方端口整理为结构匹配的签名归位，补上表实际构造/守卫定向，不再重跑无变化的固定 L1 或 CAS 故障矩阵。所有报告保留仓库外协调 run，最终 SHA 和结果在 PR 复审评论中关联。
