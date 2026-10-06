# Plan：测试支持与集成目录整理

任务 ID：81；任务名称：测试支持与集成目录整理。唯一执行依据：[Spec](three-line-09-testkit-integration.spec.md)。硬前置：#80。一个独立实现 PR 交付。

## 实施步骤

1. 按最终消费者区分包内和跨模块测试，迁支持包及 tests/integration；更新 cmd/test-feedback imports 与进程 helper。

2. 更新 scoped 固定选择、Full 分组、suites/fixture/Go tags；写 test-mapping.json 保存旧新路径、父/子测试和断言所有者。

3. 先跑 runner/守卫定向，再跑 L1 与注册 E2E 编译，核对迁移断言实际命中并删除旧路径。

实施中维护 `docs/validation/three-line-09-testkit-integration/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

本次代码路径按实际 owner 整理：`internal/coreboundary` → `internal/testkit/boundary`，`internal/testboundary` → `internal/testkit/boundary/e2e`，`internal/testfeedback` → `internal/testkit/feedback`，`internal/testprocess` → `internal/testkit/process`。#80 已把 `internal/steerlineintegration` 与 `internal/steerline/integration` 的跨模块测试迁到 `tests/integration`；本阶段只承接现有消费者，不重迁这些文件或其 assertion。执行单元候选测试 `TestMC*` 当前在 `internal/execution/candidate_workers_test.go`；独立真实 app 的 `TestMC*` 在 `tests/integration/execution_wiring_test.go`。不使用已删除的 `internal/steerline/candidate_workers_test.go` 旧路径声称 owner。所有改名 parent 保留原名；每个原/新文件、parent/subtest 和关键断言列在任务验证目录的 `test-mapping.json`。

环境：Windows；runner 与真实 app 使用受控/临时 Git 和 SQLite，不发模型请求、不设置原生 opt-in；每条命令使用独立短路径 `$env:TEMP\sl81-<run>` 作为 `STEERLINE_HOME` 并在结束时恢复。所有 runner 输出必须指向新建的空目录；保留原始 Go JSON、runner summary、编译日志、实际 parent/subtest 命中及清理诊断。命令均在固定源码提交上执行，非零退出、skip、零命中或 runner summary 不完整均不通过。

| 级别、组或场景 ID | 实际 package / parent / subtest | 环境 | 命令 | 执行时点 / 实际耗时 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- | --- |
| 定向 Go：TM09.1–TM09.3，`quick-targeted` | `./cmd/test-feedback`、`./internal/testkit/boundary/...`、`./internal/testkit/feedback`、`./internal/testkit/process`；runner 选择/Full owner 枚举、边界正负例、summary parser、Windows Job process helper 的全部 parent/subtest | Windows；无模型，process 子孙用例使用 `t.TempDir` 与 Job | `go run ./cmd/test-feedback -out docs/validation/three-line-09-testkit-integration/runner/commit-bound-02 -- ./cmd/test-feedback ./internal/testkit/boundary/... ./internal/testkit/feedback ./internal/testkit/process -count=1 -timeout=5m` | 修正首轮 root 计算后，用修复源码提交在新空目录重跑；runner 11.7s，外层 12.8s | exit 0；5 包，108/108 PASS、0 fail/skip；Full 枚举包含五个 testkit owner，边界覆盖全部源码 | `docs/validation/three-line-09-testkit-integration/runner/commit-bound-02/`；首次失败保留于 `runner/commit-bound-01/` |
| 定向真实组合：TM09.2–TM09.3，固定 `quick-boundary` 选择 | `./tests/integration`：`TestCD67CoreBoundary`、`TestCD67AdditionalImplementation`、`TestTM04ExecutionWiring`（原三个完整子测试）、`TestTM06ResourceWiring`、`TestTM07StorageWiring`、`TestTM08AppHostWiring`、`TestAppRequirementsClaimFreezesEvaluatedProfile`（default/explicit） | Windows；真实 app 构造、受控实现、隔离本地现场；无 live/native | `go run ./cmd/test-feedback -out docs/validation/three-line-09-testkit-integration/integration/commit-bound-01 -- ./tests/integration -run '^(TestCD67CoreBoundary|TestCD67AdditionalImplementation|TestTM04ExecutionWiring|TestTM06ResourceWiring|TestTM07StorageWiring|TestTM08AppHostWiring|TestAppRequirementsClaimFreezesEvaluatedProfile)$' -count=1 -timeout=15m` | 跨模块 importer/命令修订后；沿用原 quick-boundary 的 15m Go 超时及完整选择；runner 692.1s，外层 692.9s | exit 0；1 包 59/59 PASS、0 fail/skip；TM04 三原子用例，TM08 default/app host 以及 requirements default/explicit 均命中 | `docs/validation/three-line-09-testkit-integration/integration/commit-bound-01/` |
| 编译：TM09.2，已注册 E2E 场景 | `./tests/e2e/...`，`-tags=e2e`，登记的十个 `suites.json` ID / package / test / env 保持原值；`-run '^$'` 不运行场景 | Windows；构建检查，无现场、无模型 | `go test -tags=e2e ./tests/e2e/... -run '^$'` | process helper imports、boundary guard 和注册入口更新后；仅编译，4.9s | exit 0；11 个 E2E 包均编译，所有 registered 包覆盖；log 显示 `no tests to run`，不声明 E2E 行为通过 | `docs/validation/three-line-09-testkit-integration/e2e-build/commit-bound-01/` |
| L1 固定最小入口，`quick-core`、`quick-rules`、execution owner/boundary、`quick-boundary` 与 app owner | `cmd/test-feedback` 固定 selector；新 boundary/e2e、feedback/process package 名；#80 的真实 app/host/exec assertions 仍按现行映射命中 | Windows；单次隔离 `STEERLINE_HOME`，无 native opt-in、无 live | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-09-testkit-integration/l1/commit-bound-01` | 定向修复和源码提交验证后最后运行一次；12 组 execution 977.1s，外层 980.3s | exit 0；12 组/27 包，459/459 PASS、0 fail/skip；包含原 457 选择与新加的 2 条 testkit/import 守卫 | `docs/validation/three-line-09-testkit-integration/l1/commit-bound-01/` |

边界守卫继续扫描所有 OS/build tag/generated `.go` source，并构造 Windows/Linux 生产依赖图；testkit 只允许 `cmd/test-feedback` 使用 feedback/process、现有 E2E helper 使用 process，integration 不能导入 `tests/e2e/internal`。旧 `coreboundary` 正/负例及 TM04/05/06/07/08 子项保持原 assertion；新增 source-owner 检查只覆盖 Spec 已要求的窄依赖边界。跨系统只做依赖图/编译不冒充运行行为。当前 E2E 注册/环境约定见 [E2E README](../../../../tests/e2e/README.md)。

不选 Full / L2 / L3 / live：Full 的 `go list ./...` 枚举由定向 runner 回归断言覆盖；本任务聚焦 testkit 路径/owner 与固定选择，组合行为归 #82。E2E 编译不冒充场景运行，不为搬测试发模型请求。#38/#39 原生证据按未变行为复用，不接管其原生验收。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-09-testkit-integration/README.md`，注明复用证据、定向重验依据及未具备能力。

#82 得到最终守卫、非零命中映射和独立组/场景入口；保存 #38/#39 已迁路径，后继直接消费唯一新入口。
