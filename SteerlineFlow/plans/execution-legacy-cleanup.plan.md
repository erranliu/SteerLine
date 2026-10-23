# 执行域遗留清理

2026-10-16，用户直接授权的普通开发。执行与资源生命周期已迁入 SteerRunner，本次只删除 SteerLine 中无生产消费者的旧执行规则、端口和专属测试。

## 实施范围

1. 检索所有平台、build tag 和测试源码，区分外部生产引用、包内引用闭包及仅测试引用；删除前逐项核对。
2. 删除旧 lifecycle/adapter 抽象、原生事件处理、旧候选/状态转移和只验证这些实现的测试。保留宿主进程能力、checkpoint、产品展示、当前 Task/DAG/交付以及仍被消费的值类型与编解码验证。
3. 同步固定 L1 的包登记，移除已无测试的 `execution/model`；现有 DTO 守卫发现的环境检查冗余 RawMessage 别名改为直接使用标准库类型，保持 JSON 与行为不变。
4. 同步当前代码架构、生命周期归属和测试说明，结果登记[专属验收](../testing/acceptance/execution-legacy-cleanup.md)。历史冻结 Spec/Plan 和验收事实不改写。

不改变 CLI/HTTP、数据库格式、原请求对账、Runner 输入/恢复或交付行为；不移动现有包，不引入兼容、迁移或替代状态机。W01–W18、E01–E07、V01–V08 继续以 [worker lifecycle](../../docs/product-design/worker-lifecycle.md)为准，本次仅清理本仓已无调用者的旧实现。

## 测试计划

| 自动验收项 | 级别/组 | 环境、命令与时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [ELC-A1](../testing/acceptance/execution-legacy-cleanup.md#elc-a1-引用与删除范围) | 源码引用审计 | 删除前、后检索全部 Go 源码，包括平台文件、build tag 与测试；以 AST 导入别名/声明依赖辅助核对 | 删除的能力没有生产消费者；有效值和调用方可靠性边界保留 | 专属验收的删除/保留清单与引用证据 |
| [ELC-A2](../testing/acceptance/execution-legacy-cleanup.md#elc-a2-编译与定向回归) | 确定性编译及 L1 | 清理后 `go build ./...`；`GOOS=linux CGO_ENABLED=0 go build ./...`（PowerShell 临时环境）；`go run ./cmd/test-feedback -build-only`（只编译全部默认测试，不执行 Full）；`go run ./cmd/test-feedback -level=L1`；`go run ./cmd/test-feedback -- ./cmd/test-feedback ./internal/product/httpapi -count=1`；`go run ./cmd/test-feedback -- ./internal/runnerbridge ./internal/taskrunner -count=1` | Windows/Linux 生产入口编译成功；默认测试编译、L1 和受影响运行器/HTTP、原请求/PR 绑定包均 complete/passed，零失败 | 专属验收登记命令与临时 runner 报告 |
| [ELC-A3](../testing/acceptance/execution-legacy-cleanup.md#elc-a3-文档与差异) | 文档静态检查 | 文档同步后 `python scripts/check_doc_links.py`、`git diff --check` | 本次链接和差异无错误 | 专属验收 |

L2/L3/L4、真实模型、PR 和调试包不选：本次不修改运行行为或外部执行边界，确定性调用方回归及跨平台编译覆盖删除风险。无需人工窗口，不操作安装或保留运行环境。
