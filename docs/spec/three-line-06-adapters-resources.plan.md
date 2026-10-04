# Plan：执行适配与资源目录归组

任务 ID：78；任务名称：执行适配与资源目录归组。唯一执行依据：[Spec](three-line-06-adapters-resources.spec.md)。硬前置：#76。一个独立实现 PR 交付。

## 实施步骤

1. 按实际消费者决定 codexapp 协议子包归属，先迁资源包再迁三后端；同步 test subprocess、embed 和 app 工厂。

2. 迁各实现包内协议/配置/绑定代表与外层接线；保留权限/发布责任和恢复原身份。

3. 更新目录/runner/文档路径，删除旧路径并记录 native-impact.md 对 #39 每个受影响入口的复用/重验决定。

实施中维护 `docs/validation/three-line-06-adapters-resources/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM06.1–TM06.2 | Windows；协议/资源 fixture，无真实模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/adapters -- ./internal/execution/adapters/codex ./internal/execution/adapters/orca ./internal/execution/adapters/orcaremote -run '^TestTM06(Protocol\|FrozenBinding\|Unsupported)$' -count=1` | 各后端切换后 | 迁入每实现至少一个原协议/冻结绑定/拒绝代表，非零命中 | `docs/validation/three-line-06-adapters-resources/adapters/` |
| 定向 Go：TM06.1–TM06.2 | Windows；真实 app 与隔离 Git/可控进程 | `go run ./cmd/test-feedback -out docs/validation/three-line-06-adapters-resources/wiring -- ./internal/steerlineintegration -run '^TestTM06ResourceWiring$' -count=1` | 资源和默认构造接通后 | 默认实现、实际资源模板与恢复原目标一致 | `docs/validation/three-line-06-adapters-resources/wiring/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-06-adapters-resources/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-06-adapters-resources/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不默认 Full/L2/L3/live：纯搬包复用当前原生证据。如真实启动/权限/协议改变，本 Plan 修订为具体受支持平台最小原生命令与原身份回执证据后执行，不用 fake 宣称通过；远端缺口继续阻塞 #39。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-06-adapters-resources/README.md`，注明复用证据、定向重验依据及未具备能力。

#80 接新工厂与资源小端口；#39 记录固定构建，迁移后只重验 native-impact.md 中发生实际变化的入口。
