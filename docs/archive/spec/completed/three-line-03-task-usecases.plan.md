# Plan：任务应用用例归包

任务 ID：75；任务名称：任务应用用例归包。唯一执行依据：[Spec](three-line-03-task-usecases.spec.md)。硬前置：#74。一个独立实现 PR 交付。

## 实施步骤

1. 按 #74 清单迁创建/文档校验和依赖用例，迁其包内单元测试；抽出任务最小配置输入。

2. 迁挑选、结果应用、丢弃意图，保持原事务声明；切换 app、CLI 和执行消费者。

3. 删除原主包任务方法和转发，更新守卫及受影响 L1 路径。

实施中维护 `docs/validation/three-line-03-task-usecases/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

本次实现已接通 `task.Commands`、`task.Service` 与 `TestTM03*` 精确目标。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。既有多候选/结果专用提交由 #74 证据与本次 L1 保留，本次不重建其矩阵。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM03.1–TM03.3 | Windows；纯任务值与注入窄端口，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/task -- ./internal/task -run '^TestTM03(TaskBatch\|Dependencies\|Result)$' -count=1` | 每类任务入口迁移后 | 迁入严格解码/原回执重放、依赖终态拒绝、固定值/结果断言；SQLite 回滚见下行 | `docs/validation/three-line-03-task-usecases/task/` |
| 定向 Go：TM03.2 事务 | Windows；临时 Git/SQLite、受控执行 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/transactions -- ./internal/steerline -run '^(TestTaskBatchDeterministicCyclePath\|TestCreateTaskBatchAtomicIdempotentAndQueryable\|TestCreateTaskBatchRollsBackEveryWriteWhenEdgeInsertFails\|TestNativeCommandsRejectConfigurationBindingDriftBeforeSideEffects\|TestTaskBatchRejectsDuplicateAndCrossProjectDependencies\|TestAddNativeTaskPredecessorsIsAtomicIdempotentAndBlocksClaim)$' -count=1` | 新事务适配接通后 | 保留实际数据库全写回滚、请求重放、排序、重复/跨项目/环/版本拒绝与 reopen 断言；审计失败无成功回执 | `docs/validation/three-line-03-task-usecases/transactions-20261004/` |
| 定向 Go：TM03.1 | Windows；真实 app 与 CLI 构造，可控执行端口 | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/wiring -- ./internal/steerlineintegration -run '^TestTM03TaskWiring$' -count=1` | 默认消费者切换后 | 真实产品与 CLI 调用 task；原回执/同请求重试、审计失败原子回滚、新文件核验能力的 SHA/regular-file 拒绝；产品无结果写能力 | `docs/validation/three-line-03-task-usecases/wiring/` |
| 定向 Go：TM03.2 固定文档 | Windows；临时 Git/SQLite | `go run ./cmd/test-feedback -out docs/validation/three-line-03-task-usecases/documents -- ./internal/steerline -run '^TestTaskContract(PinsCommittedBlobsAndRejectsInvalidReferences\|RejectsCommittedSymlinkReferences)$' -count=1` | 内容规则迁入 model 后 | 保留原 canonical digest、固定 commit/路径/标题与 regular-file 断言；新 task 文档能力另由 wiring 实测 | `docs/validation/three-line-03-task-usecases/documents-20261004/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-03-task-usecases/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-03-task-usecases/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../../../tests/e2e/README.md)。

不选 Full/L2/L3/live：任务用例迁包，规则/事务代表与 app 接线足够；执行副作用与产品界面不改。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-03-task-usecases/README.md`，注明复用证据、定向重验依据及未具备能力。

#79 接任务消费者端口，#80 接剩余组装；与 #76 并行只改 task 所有文件，共同 cmd/app 段顺序集成。

## 实际构造与验证范围

最小任务输入/文档端口、Commands 的任务事务投影、TaskService 固定文件核验能力及 #76 共同接线要求见 [interface-handoff](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/three-line-03-task-usecases/interface-handoff.md)。当前创建/追加依赖生产路径已经直接消费 task.Commands；已迁的 Ready/Fixed/Claim/Declare/Intent/Discard 唯一实现保持。领取仍保留主干实际核验消费者，#76 在迁消费方端口时按交接改为 VerifyContractFiles 并删旧函数；不保留已删函数调用、不用默认 no-op 核验，#75 不重写共同执行文件。

Task 单元环境使用窄端口以保持全 OS/tag 的 task 纯依赖守卫；实际 Git/SQLite/CLI 行为在外层事务与 wiring 测试验证。无新增 E2E 环境/场景能力。Windows/Linux 显式 `go list -deps -json ./internal/task` 和 `go build ./cmd/steerline ./internal/task` 检查直接/传递依赖与可构建性，产物在本目录 task-deps-{windows,linux}.json；跨系统仅为静态/编译证据。固定 L1 同时覆盖既有 source 负向注入和 Windows/Linux 生产图，不改 coreboundary 守卫或 scoped 公共选择。
