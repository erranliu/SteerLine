# 候选答复投递和等待状态

2026-11-12，普通开发，受测基线 `40d710fb89d21923c2a107399b582cd717c3709a` 加本次工作区修改。

`merge-h-308-code` Task 18 的原答复有 2220 个 UTF-8 字节，超过 2000 字节上限。原 Web 请求保留了正文，校验错误未落拒绝回执。原轮次完成后 Runner 状态为 `waiting_input`，候选详情按步骤前缀显示「实施中」。

- 编辑区显示实际投递的 UTF-8 字节数，包括选项文案和结构化回答的 JSON 编码。超限时显示超出字节数，禁用提交并保留原草稿。直接提交到 Web 后端的超限请求持久化 `rejected / answer_too_large`，按原请求 ID 查询或重放均返回同一拒绝回执。
- 候选详情优先读取 Runner 状态，`waiting_input` 显示「等待输入」。原活动轮次继续运行时仍按实际步骤显示。

## SteerAgent 提示词

在固定规则中增加一段。原问题材料的 `question` ID 来自自动回答建议的正式调用方，包含以结果陈述形式出现的待答事项。生成稿会预填到同一个答复入口，因此同时说明字数要求和该入口的 UTF-8 字节上限。

> 当 materials 包含 id 为 question 的原问题材料时，直接针对其中的待答事项拟写可提交的精简回答。保留关键决定、必要条件和待确认事项，省略重复背景。回答最多 1500 字，且最多 2000 个 UTF-8 字节；超出任一上限时先压缩再输出。

| UTF-8 内容 | 修改前 | 修改后 |
| --- | ---: | ---: |
| 短模板 | 145 | 145 |
| 固定规则 | 1025 | 1329 |
| 黄金组装正文 | 1180 | 1484 |
| 含陈述式待答事项的完整 advisory-v1 信封样例组装 | 1420 | 1724 |

独立审查核对了真实材料来源、Runner 指令接线、完整正文、黄金样本和规则摘要，复审通过。

## 验证

- `go test ./internal/prompts -count=1 -v` 通过，实际渲染的 1484 字节与黄金逐字节一致。
- L1 通过。报告 `C:/SteerlineDebug/wt-f891-answer-delivery/test-results/L1/20261112T023525563Z-14c6cc1961ee420291cf79b861b428d3/summary.json`。
- L2 `smoke.delivery` 通过，`cleanup_complete=true`。报告 `C:/SteerlineDebug/wt-f891-answer-delivery/test-results/L2/20261112T023525563Z-14c6cc1961ee420291cf79b861b428d3/summary.json`。
- 前端答复、建议、阶段映射和候选页面 44 项通过，生产构建通过。扩大候选测试范围时发现环境接口夹具未返回 `environments` 数组，修正三个测试夹具后复验。原始 Vitest JSON 和构建日志位于 `C:/SteerlineDebug/wt-f891-answer-delivery/diagnostics/20261112T023942652Z-5f1db3923d234c22aacfcbcdd3c0f540/`。
- 后端定向验证通过，包含 2000 字节边界、超限拒绝回执的提交/查询/重放，以及 SteerAgent 从原问题自动触发和恢复。报告 `C:/SteerlineDebug/wt-f891-answer-delivery/diagnostics/20261112T023820131Z-28720e2fcd4247d1a4280e31531eee0b/summary.json`。
- L4 HTTP 专项 `go.http` 通过。报告 `C:/SteerlineDebug/wt-f891-answer-delivery/test-results/L4/20261112T023910859Z-51b6113b6753404193f0975d2cd18136/summary.json`。

安装版和原 Task 18 的请求未改动。模型对新增字数要求的遵循情况未做真实模型验收。
