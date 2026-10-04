# Plan：独立开发证明与最终清理

任务 ID：82；任务名称：独立开发证明与最终清理。唯一执行依据：[Spec](three-line-10-independence-cleanup.spec.md)。硬前置：#81。一个独立实现 PR 交付。

## 实施步骤

1. 核对 #73–#81 实际所有者和残留，列 deletion-inventory.md；只删无消费者路径并保留断言映射。

2. 挑三项真实独立改动及证据，缺口用隔离验证补丁经生产入口证明后撤回；写 independence.md 逐项说明变更文件/端口/结果。

3. 更新当前代码架构全 OS 索引、目录/安装/runner 文档，跑最终最小组合并汇总限制；不修改 #39/#42/#53 产品汇合条件。

实施中维护 `docs/validation/three-line-10-independence-cleanup/ownership.md` 和 `test-mapping.json`：记录实际旧/新文件、类型/端口、构造消费者及迁入测试父/子名称、断言所有者。前置已迁的定义/接线不重复迁移。共享文件按 Spec 的唯一修改所有者顺序集成，每步消费者与测试立即可运行。

## 测试计划

以下新包及 `TestTM*` 是实施时需要接通的精确目标，不表示本次文档 PR 已创建测试或入口。优先将已有代表测试迁到真正所有者并保留断言，只补 Spec 的未覆盖缺口；不能新增仅镜像实现的测试。迁移影响命令时在同次实现中修订到唯一实际包，记录旧/新 parent/subtest，零匹配或 skip 不算通过。TestMC* 当前来源是 `internal/steerline/candidate_workers_test.go`。

环境：Windows 临时 Git/SQLite/受控后端，STEERLINE_HOME 指向本轮隔离目录并在结束后恢复；不设置原生 opt-in。下表输出路径为证据目录基名，每次使用新的空运行子目录，不覆盖前置报告。原始 Go JSON、runner summary、身份/版本/原请求回执和必要清理结果一并保留。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：TM10.1 | Windows；真实业务注入/产品 fixture，无模型 | `go run ./cmd/test-feedback -out docs/validation/three-line-10-independence-cleanup/independence -- ./tests/integration -run '^TestTM10(TaskSelection\|ExecutionRule\|ConsoleInteraction)$' -count=1` | 三条线证明补丁固定时及撤回后 | 补丁阶段触发预期差异/单线修改，撤回后原行为通过；测试非零命中 | `docs/validation/three-line-10-independence-cleanup/independence/` |
| L3 delivery.basic：TM10.3 | Windows；controlled，临时 Git/SQLite/provider，无模型 | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=controlled '-scenario=delivery.basic' -out docs/validation/three-line-10-independence-cleanup/delivery` | 最终清理与 L1 通过后 | 实际产品入口唯一 Delivery/后继 base 成立，环境清理完成 | `docs/validation/three-line-10-independence-cleanup/delivery/` |
| L3 recovery.waiting-merge-restart：TM10.3 | Windows；controlled，真实隔离服务重启，无模型 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.waiting-merge-restart' -out docs/validation/three-line-10-independence-cleanup/recovery` | 最终构建稳定后 | 重接原身份/原请求并完成真实受控提交，清理证据齐全 | `docs/validation/three-line-10-independence-cleanup/recovery/` |
| L1 固定最小入口 | Windows；隔离 STEERLINE_HOME，无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-10-independence-cleanup/l1` | 自查/定向修复完成后一次 | 现有规则/构造/守卫代表非零命中且通过，无意外 live | `docs/validation/three-line-10-independence-cleanup/l1/` |

边界类变更在本阶段同步既有守卫/负向注入及 L1 选择；扫描全 OS/build tag 源码，核对 Windows/Linux 生产直接及传递依赖。跨系统只做依赖图/编译不冒充运行行为。新增场景或环境能力须先登记包/测试/ID与命令，再执行；当前已注册 E2E 入口和环境约定见 [E2E README](../../tests/e2e/README.md)。

不选 Full/L2/live：各前置已做定向与 L1，最终只补交付/重启组合；真实候选和版本/长期运行仍由 #39/#42 负责。不存在新注册 E2E 场景或环境能力。

## 自查与交付

逐项核对 Spec 验收，保留提交屏障、同快照、身份/版本和唯一结果，删除迁出路径/无消费者桥接，核对最小命令实际命中。汇总 `docs/validation/three-line-10-independence-cleanup/README.md`，注明复用证据、定向重验依据及未具备能力。

交付实际最终架构、三条独立开发证明和仍未具备的原生限制；产品后续可在各自 Spec 的前置满足后接入，不新增重构总门槛。
