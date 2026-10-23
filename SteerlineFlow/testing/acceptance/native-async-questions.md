# 原生异步问答验收

归属本次普通开发任务；[Spec](../../spec/native-async-questions.spec.md)、[Plan](../../plans/native-async-questions.plan.md)。

| ID | 类型 | 条件 | 结果 / 证据 |
|---|---|---|---|
| AQ-A1 | 自动 | 原生异步问题及选项立即识别，固定来源身份、去重和恢复重放；不得成为 terminal final | 自动通过；见下方证据 |
| AQ-A2 | 自动 | 运行中问题保持 running；报告保留原问题/选项；明确完成后失效；SQLite 重读保持内容 | 自动通过；见下方证据 |
| AQ-A3 | 自动 | 保存答案、活动轮次 steer、准确回执、未知结果只读核对；停止轮次拒绝 steer、后续输入绑定来源 | 自动通过；见下方证据 |
| AQ-A4 | 自动 | 控制台运行状态与问题状态分开，运行中可编辑选项；查询正确派生阻塞状态 | 自动通过；见下方证据 |
| AQ-A5 | 自动 | 原有同步问答、答案事务、阶段转换与固定发布上下文定向回归通过 | 自动通过；见下方证据 |
| AQ-M1 | 人工 | 新建独立 live SteerlineTest 环境，真实模型发出 async 问题，控制台选择答案并核验执行中送达及轮次结束后继续 | 待验；人工窗口待定，后置且不阻塞本次代码交付 |

安装环境 101 仅用于前期只读取证，未作为调试或测试现场。临时调试包由 `scripts/new-debug-release.ps1` 生成于 `C:/SteerlineDebug/dev-20261010-163230-97ece424`，未启动服务；定向测试使用既有隔离夹具。

## 自动验证结果（2026-10-10）

- `async-questions-final/summary.json`：5 个包、10 项测试通过，0 skip，36.686 秒；原生 async 结构化选项、恢复重放、观察到问题后继续原轮次、steer 精确回执与停止后拒绝、SQLite 持久化、报告不误确认答案及来源步骤继续、控制台/API/客户端交互类型。
- `async-regression-final/summary.json`：原有同步问答、答案事务与幂等、原轮次启动重放、固定发布归属和阶段转换通过，0 skip，27.243 秒。
- `async-final-verified/summary.json`：最终边界增量（含成果不匹配时保留问题）及同步回归共 29 项，28 项通过、1 项控制台状态文案失败，0 skip，57.075 秒。失败原因是状态格式化函数返回英文 `running`；修复为实际业务状态的中文名称后，仅定向重验受影响控制台。
- `async-console-final/summary.json`：上述控制台修复后 1 项通过，0 skip，2.795 秒。其余 28 项所覆盖实现未再变化；不把先前失败的合并运行标记为全绿。
- `git diff --check` 通过。未执行 full、真实模型或真实 PR；AQ-M1 保持待验。

以上证据根为 `C:/SteerlineDebug/dev-20261010-163230-97ece424/evidence/`。保留每次原始 Go JSON 日志、stderr、源码状态和 summary.json；早期实现中失败的定向尝试已用于发现存储准入及测试现场准备缺口，不作为通过证据。

提 PR 前同步最新主干 `03df9d14`，无冲突。重验 `TestWorkerPrompt.*` 与 `TestWorkerStageTransitions`：4 项通过、0 skip，14.87 秒；证据 `async-pr-rebase/summary.json`。
