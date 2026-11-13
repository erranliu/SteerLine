# 候选回答建议只使用原问题

2026-11-13，普通开发，基线 `265edede52ff4c3d166d5d79642bd5a18f0f0a1d` 加工作区修改。Task 17 的阶段总结经建议生成后，随答复向执行 Agent 投递了额外的版本核对和证据对账要求。排期问题也携带了整套项目材料。

新建议使用冻结的原问题、选项及原生题集。阶段结果、测试安排、公共约束、验收记录、Spec 和 Plan 由原流程持有。用户确认的自由文本通过既有回答入口原样投递。已冻结的建议请求和历史结果按原记录读取。

固定规则将澄清和核实要求替换为只回答当前问题、标明缺少的用户信息、去掉背景复述和额外执行指令。实际正文差异见 [规则资产](../../../internal/prompts/materials/main-agent.md) 和 [组装样本](../../../internal/prompts/testdata/main-agent.golden.md)。独立复审核对了实际 Runner 指令接线、正文差异、摘要、黄金样本和受影响断言。

| UTF-8 正文 | 修改前 | 修改后 |
| --- | ---: | ---: |
| 短模板 | 145 | 145 |
| 完整规则 | 1329 | 1300 |
| 黄金组装正文 | 1484 | 1455 |
| Task 17 阶段结果待答请求 | 67404 | 3817 |
| Task 17 T92 排期待答请求 | 66082 | 2495 |

Task 17 的修改前数值来自已持久化的 Runner 指令，修改后数值为同一原问题在新规则和组装方式下的离线重组。未向原 Run 投递，也未在真实模型上重新生成。

## 验证

- `go test ./internal/prompts -count=1 -v` 通过，渲染正文和黄金样本逐字节一致。
- App 与 taskrunner 定向回归通过。持久候选的公开 Runner 请求只有原问题材料，自由文本和原生答案按既有入口投递。报告 `C:/SteerlineDebug/wt-ef7a-reply-80235c42/diagnostics/answer/summary.json`。
- L1 和 L2 `smoke.delivery` 通过。报告为 `C:/SteerlineDebug/wt-ef7a-reply-80235c42/test-results/L1/verification/summary.json` 和 `C:/SteerlineDebug/wt-ef7a-reply-80235c42/test-results/L2/verification/summary.json`。首次沙箱运行因临时 Git 写入和报告原子替换权限失败，使用独立测试目录复验。
- L4 `runtime.main-agent-advisory` 通过，覆盖实际 Runner 请求、浏览器展示、恢复和取消。报告 `C:/SteerlineDebug/wt-ef7a-reply-80235c42/test-results/L4/advisory/summary.json`。
