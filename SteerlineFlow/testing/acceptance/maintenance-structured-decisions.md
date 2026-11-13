# 维护只读决策结果校验

2026-11-13，基于主干 `1a24ca78` 验证本次改动。维护仅采用原只读请求对应的成功 `ContentResult.JSON`。Runner 结果通过既有 Run、Final 来源、目录及冻结材料核验后，由决策核心校验，再由维护业务按阶段和预算决定是否写入。Final 文本保留为反馈。缺少结构化结果时结算原只读操作并记录阻断。

## 验证

| 范围 | 结果与证据 |
| --- | --- |
| `go test ./internal/refactoring ./internal/steeragent/maintenance -count=1` | 通过。缺失、空值和业务校验不通过的结构化结果均不能使用合法 Final 决策文本推进写入。原 Final 反馈仍可读取。来源错配、Runner 失败后的单次重试、正常轮次及美元预算收尾均通过。 |
| L1 `go run ./cmd/test-feedback -level=L1` | 通过。报告为 `%TEMP%/steerline-structured-decision-L1-7c4fd9d6c0bf4e01bf84628b667b6372/summary.json`。 |
| L2 `go run ./cmd/test-feedback -level=L2` | 通过。报告为 `%TEMP%/steerline-structured-decision-L2-7f81a9d1210941d9b14b94df847fdcb0/summary.json`。 |
| L3 `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.refactoring-maintenance` | 通过。报告为 `%TEMP%/steerline-structured-decision-L3-27854b4f82b64b8fa579c79c6aca1e86/summary.json`。覆盖两轮维护、独立审计、原 PR、同一工作区、浏览器与资源清理。 |

本轮使用受控 Runner 夹具，未运行真实模型或部署。
