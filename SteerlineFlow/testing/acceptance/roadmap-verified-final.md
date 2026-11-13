# Roadmap 结构化结果的 Final 来源校验

2026-11-13，基于主干 `265edede` 验证工作区改动。Roadmap 解析和单任务文档生成共用结果接收入口。原 Run 完成并停止后，接收成功的 `ContentResult.JSON` 前核对结果来源与原 Run 已验证的 `Final` 完全一致。缺少 Final、来源错配或 JSON 为空时记录无效结果，不采用解析或文档。已验证的结果继续进入各自业务校验和审阅。

## 验证

| 范围 | 结果与证据 |
| --- | --- |
| `go test ./internal/roadmap -count=1` | 通过。解析与文档生成均覆盖 JSON、冻结材料和仓库来源有效、仅结果 TurnID 错配时无法进入审阅，以及来源一致时正常审阅。解析另外覆盖缺少 Final、Final 未验证及 JSON 为空。 |
| L1 `go run ./cmd/test-feedback -level=L1` | 通过。报告为 `%TEMP%/steerline-roadmap-final-L1-edee77b48f2f484f92a1412286219f7f/summary.json`。 |
| L2 `go run ./cmd/test-feedback -level=L2` | 通过。报告为 `%TEMP%/steerline-roadmap-final-L2-f5b205748ed34364beeb1995da79ede3/summary.json`。 |
| Roadmap 专项 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.roadmap` | 通过。报告为 `%TEMP%/steerline-roadmap-final-roadmap-a8d3c3fdff5146fbae73d361a3dd9e59/summary.json`。覆盖解析、文档生成与重试、真实本地 Git 发布和入图、CRLF 检出及恢复。 |

未运行真实模型或部署。
