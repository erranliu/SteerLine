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
| 定向 Go：TM02.1 | Windows；隔离 SQLite、可控后端 | `go run ./cmd/test-feedback -out docs/validation/three-line-02-task-execution-boundary/claim -- ./internal/steerline -run '^(TestMC1\|TestMC2\|TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation\|TestNativeClaimAllowsManualParallelButKeepsAutoSlot)$' -count=1` | 真实启动切换后 | 保留首次/后续身份与领取/模式代表断言，全部非零命中 | `docs/validation/three-line-02-task-execution-boundary/claim/` |
| 定向 Go：TM02.2–TM02.3 | Windows；隔离 SQLite/provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-02-task-execution-boundary/result -- ./internal/steerline -run '^(TestMC5\|TestMC6\|TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor\|TestTaskDiscardRequestPersistsUnknownAndThenDiscardsOnlyAfterUnmergedProof\|TestTM02ResultCAS)$' -count=1` | 结果和丢弃同事务接通后 | 已有交付/冲突代表通过；补结果去重、旧版本及交付/丢弃竞争缺口 | `docs/validation/three-line-02-task-execution-boundary/result/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-02-task-execution-boundary/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-02-task-execution-boundary/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：身份/事务责任变化由受控实际生产接线验；不重跑原生权限矩阵。L3 组合留 #82；若新发现恢复路径受影响则只追加 recovery.waiting-merge-restart。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-02-task-execution-boundary/README.md`，注明复用证据、定向重验依据及未具备能力。

#75/#76/#77 只能消费本次已接通的窄端口；清单指定共同文件集成所有者，不把接口草案当交付。与 #39 并行时固定其构建，变更交付路径只定向复核受影响证据。
