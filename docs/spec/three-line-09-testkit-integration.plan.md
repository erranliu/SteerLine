# Plan：测试支持与集成目录整理

任务 ID：81；任务名称：测试支持与集成目录整理。唯一执行依据：[Spec](three-line-09-testkit-integration.spec.md)。硬前置：#80。一个独立实现 PR 交付。

## 实施步骤

1. 按最终消费者区分包内和跨模块测试，迁支持包及 tests/integration；更新 cmd/test-feedback imports 与进程 helper。

2. 更新 scoped 固定选择、Full 分组、suites/fixture/Go tags；写 test-mapping.json 保存旧新路径、父/子测试和断言所有者。

3. 先跑 runner/守卫定向，再跑 L1 与注册 E2E 编译，核对迁移断言实际命中并删除旧路径。

实施中维护 `docs/validation/three-line-09-testkit-integration/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM09.1–TM09.3 | Windows；测试工具/禁止依赖反例，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-09-testkit-integration/runner -- ./cmd/test-feedback ./internal/testkit/boundary -count=1` | 工具和选择迁移后 | 原 runner 选择拒绝/边界负例通过；生产/test/E2E 依赖不放宽 | `docs/validation/three-line-09-testkit-integration/runner/` |
| 定向 Go：TM09.2–TM09.3 | Windows；真实 app 及受控实现 | `go run ./cmd/test-feedback -out docs/validation/three-line-09-testkit-integration/integration -- ./tests/integration -run '^(TestCD67CoreBoundary\|TestTM08AppHostWiring)$' -count=1` | 跨模块迁移后 | 代表父/子用例非零命中，与映射原断言一一对应 | `docs/validation/three-line-09-testkit-integration/integration/` |
| 编译：TM09.2 | Windows；E2E tag，无场景执行 | `go test -tags=e2e ./tests/e2e/... -run '^$'` | 注册表与路径更新后 | 已注册受控/原生包可编译；仅构建证据，不算行为通过 | `docs/validation/three-line-09-testkit-integration/e2e-build.log` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-09-testkit-integration/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-09-testkit-integration/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/L3/live：此任务验证测试路径/选择与覆盖，行为组合归 #82；编译不会冒充 L3 通过，不为迁目录发模型请求。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-09-testkit-integration/README.md`，注明复用证据、定向重验依据及未具备能力。

#82 得到最终守卫、非零命中映射和独立组/场景入口；保存 #38/#39 已迁路径，后继直接消费唯一新入口。
