# Plan：执行应用用例归包

任务 ID：76；任务名称：执行应用用例归包。唯一执行依据：[Spec](three-line-04-execution-usecases.spec.md)。硬前置：#74。一个独立实现 PR 交付。

## 实施步骤

1. 按 #74 清单迁执行状态和局部用例，接 task 端口及独立输入；迁包内测试，不并行重写 #75 文件。

2. 迁 actor 单步协调、会话/Decision 与恢复，再迁交付核验与清理；每步完成生产构造和删除旧路径。

3. 保留注入的宿主端口，更新应用及 runner/守卫路径；记录 #39 证据影响，不改权限或原生协议。

实施中维护 `docs/validation/three-line-04-execution-usecases/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM04.2–TM04.3 | Windows；受控后端/SQLite/provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-04-execution-usecases/candidates -- ./internal/steerlineintegration -run '^TestMC[1356789]$' -count=1` | 候选/提交/恢复迁移后 | 原 MC 改为真实 app 的外层跨模块断言；身份、交付、放弃、阻断与晚到副作用非零命中 | `docs/validation/three-line-04-execution-usecases/candidates/` |
| 定向 Go：TM04.1–TM04.2 | Windows；真实 app，提交失败与原请求对账 fixture | `go run ./cmd/test-feedback -out docs/validation/three-line-04-execution-usecases/wiring -- ./internal/steerlineintegration -run '^TestTM04ExecutionWiring$' -count=1` | actor 和默认工厂切换后 | 调用真实新执行入口；原 intent/receipt 及失败回滚保留 | `docs/validation/three-line-04-execution-usecases/wiring/` |
| 定向 Go：重建/Journals/TM04 边界守卫 | Windows；孤立记录 fixture，无 SQL/adapter writer，coreboundary 编译 Windows/Linux 依赖图 | `go run ./cmd/test-feedback -out docs/validation/three-line-04-execution-usecases/reconstruction -- ./internal/execution/reconstruction ./internal/coreboundary ./internal/execution -run '^(TestBuildOwnsRecordsAndMetadataAcrossRefreshes|TestProjectionRejectsBrokenBindingsWithoutReusingOldViews|TestProjectionRetainsDeliveryAndReservationRules|TestExecutionProjectionKeepsRemotePathAndWhitespaceSemantics|TestTM04ExecutionBoundary|TestJournal.*)$' -count=1` | 重建包、JournalCursor 与守卫迁移后；L1 前 | 4 个纯重建断言、边界父/反例及全部 journal parents 命中；无 skip | `docs/validation/three-line-04-execution-usecases/reconstruction/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level L1 -out docs/validation/three-line-04-execution-usecases/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-04-execution-usecases/l1/` |
| L3 controlled：`recovery.waiting-merge-restart` | Windows；已注册的受控恢复 backend | `go run ./cmd/test-feedback -level L3 -suite recovery -env controlled -scenario recovery.waiting-merge-restart -out docs/validation/three-line-04-execution-usecases/controlled` | 通用 host loop 与 actor 单步提取后 | 原 task/DB/workflow/execution/worktree 身份稳定、不重派并正常交付 | `docs/validation/three-line-04-execution-usecases/controlled/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：平台协议与权限不变，重用既有原生证据。actor/host loop 拆分触及恢复宿主，故本次追加一次已注册的 L3 controlled `recovery.waiting-merge-restart`；最终组合归 #82。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-04-execution-usecases/README.md`，注明复用证据、定向重验依据及未具备能力。

#78 得到执行/backend 固定接口，#79 得到声明提交与读取需求，#80 得到宿主端口；#39 不能用迁移前证据宣称迁移后改变的行为通过。
