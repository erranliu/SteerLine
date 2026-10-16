# Plan：测试分级与 L4 入口

普通开发，按用户授权直接实施，不进入 roadmap。采用 [测试架构规范](../../SteerlineProjectGuideDoc/testing-architecture.md)，专属验收见 [记录](../acceptance/test-level-alignment.md)。仅修改测试及调度基础设施，不改变产品 worker/恢复行为；原职责沿用 [生命周期](../product-design/worker-lifecycle.md)。

## 实施范围

1. L2 两阶段保持原样；L3 为等待合并时 SteerLine 重启、正常宿主生命周期、双 home 独立操作。原 runtime 四项改级 L4，分别保留构建/路径/宿主拒绝/同 ID 重放的独有断言；从混合场景提取两个 L3，不为边界参数新增完整流程。
2. `test-feedback` 支持 L4 controlled、多选 suite/scenario、同级选择、精确报告和原证据/期限/清理核验。L3 支持跨包并行，总并行预算为二：两个包并行时各包串行，一个包时允许两项并行；L4 当前按包顺序执行，runtime 包内最多两项并行，因为这些边界现场相互隔离。
3. 更新独立 Windows 进程脚本，可选择 L3/L4，默认完整运行两级 runtime，不再将四项边界混入 L3。所有实际场景用原分级隔离夹具，不使用安装目录；受工具 Job 影响时沿用独立 Windows 进程上下文。
4. 对照 Runner 已提交源码和场景清单，等价执行专项可删除；SteerLine caller/Task/事务/宿主/路径规则保留。此前已迁出的测试不重复计作本次删除。同步清单、导航和验收记录，不改历史证据。

## 测试计划

| 验收项 | 级别/场景或组 | 环境、命令与时点 | 通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| TLA-A1 | test-feedback 选择/调度/证据 owner | 实施后 `go test ./cmd/test-feedback -count=1` | L4 接受合法选择，跨级/重复/未知/错误环境拒绝；L2 gate 保持；并行无丢项，失败、取消与清理正确报告 | 专属验收 A1，临时 JSONL |
| TLA-A2 | L3：recovery/runtime 全三项 | 实施后独立进程 `go run ./cmd/test-feedback -level=L3 -suite=recovery,runtime -env=controlled` | 三项实际通过、并行选择生效、无 skip、证据及清理完整 | 专属验收 A2，临时 summary/scenario |
| TLA-A3 | L4：runtime 全四项 | 实施后 `scripts/test-runtime-isolation.ps1 -Level L4` | 四项实际通过，原边界断言保留，selected-L4、资源清理完整；实际 build 使用本项独立源码克隆，不写当前 checkout 或安装目录 | 专属验收 A3，临时 summary/scenario |
| TLA-A4 | 分级基础设施/依赖边界/文档 | 实施后 `go test ./tests/e2e/internal/... ./internal/testkit/boundary/... -count=1`；`go test -tags=e2e ./tests/e2e/... -run '^$'`；`python -X utf8 scripts/check_doc_links.py`；`git diff --check` | owner 实际命中；tag 编译通过但不计业务通过；清单与九项登记/测试映射一致，链接有效 | 专属验收 A4，临时日志 |

不运行 Full、完整 L1 或真实 L2：变更集中于分级选择与受控场景，定向 owner、实际 L3/L4 足以验证；没有改变 live 代码或模板，L2 固定选择/gate 由 A1 校验。无新增人工验收。运行失败保留现场，修复后新建独立运行，结果只更新验收记录。
