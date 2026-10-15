# 控制台候选操作精简验收

> 2026-10-15 TUI 退役：本文保留原交付/验收事实，TUI 专属操作和未验项不再适用，不记为通过。共享后台/API 要求继续归原记录；当前范围见 [TUI 退役](../spec/tui-retirement.spec.md)。

2026-10-13 普通开发，用户明确要求完整删除放弃候选功能，任务提示只显示中文状态，并增加候选数量列。代码基线为 `0b7652acafe9d0a860c174b7d2f6f40d73261781` 的本次工作区修改；未替换安装环境。

## 范围与结果

| 项 | 通过条件 | 验证入口与结果 |
| --- | --- | --- |
| CC-A1 移除放弃 | 无 z 入口、确认框、CLI reason 参数、API/control action、服务端命令及 Abandonment 专用字段；旧 action 在外发业务副作用前拒绝 | `TestCandidateListCountAndRemovedExit`、`TestAPIOriginalCommandReceipt`、`TestControlRejectsRemovedCandidateExit`；控制台/API/服务端拒绝检查已通过 |
| CC-A2 中文任务提示 | 主列表不再拼接 open/in_progress/closed 等原枚举，明确阻塞仍按事实显示；原值在详情保留 | `TestTaskListUsesBlockerFactsAndAlignedSingleRows`、`TestCompletedTaskShowsRawStateAndHistoricalExecution`、控制台包；通过 |
| CC-A3 候选数量 | 同一快照按 Task 统计全部 WorkflowRun，包含已结束候选，不混入其他任务；零候选显示 0；窄/宽屏列对齐 | `TestCandidateListCountAndRemovedExit`、`TestTaskListUsesBlockerFactsAndAlignedSingleRows`；通过 |
| CC-A4 保留其他业务流程 | 候选新增、原会话反馈、唯一交付、Task 丢弃及资源保留仍通过既有隔离夹具 | MC1–MC7/MC9、候选 PR/恢复、Control 已逐项通过；PS3 与 TaskStartControl 独立补验均 passed/complete |

SQLite schema 更新为 19，服务 checkpoint/runtime 更新为 21，拒绝旧格式且不迁移，见[数据模型](../product-design/data-model.md)。`abandoned` 工作流终态仍用于 Task 丢弃；单候选放弃字段和命令已删除。

## 自动验证与证据

Windows/amd64、Go 1.26.4，均使用既有隔离测试夹具，无真实模型或真实 PR；不使用 `C:\SteerlineRuntime`。未选择 L2/L3 或 Full：本次属于控制/展示契约精简，相关包与候选/交付定向回归覆盖自动项。

1. `go run ./cmd/test-feedback -- ./internal/console ./internal/product/api ./internal/product/httpapi ./internal/product/query ./internal/execution/model ./internal/execution/reconstruction ./internal/storage/sqlite ./cmd/steerline -count=1`：passed/complete，8 包、353 项、0 失败/跳过；164.760 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T102806.000480100Z/summary.json`。
2. 最后仅调整 Task 丢弃的中文标签、强化数量断言及退休资源注释后，执行 `go run ./cmd/test-feedback -- ./internal/console -run '^(TestCandidateListCountAndRemovedExit|TestTaskList|TestWorker)' -count=1`：passed/complete，7 项、0 失败/跳过，6.530 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T103145.533759000Z/summary.json`。其余上述包实现未再改动。
3. `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestMC[1-79]|TestPS3|TestControl|TestStartTaskControlPayload|TestTaskStartControl|TestCandidate(OpenPR|ClosedPR|ResumeError)|TestTM04)' -count=1`：613.103 秒后因整个 Go 包的 10 分钟累计期限返回 incomplete，不能记为整体通过。MC1–MC7/MC9、三个候选 PR/恢复测试和三个 Control 测试均已有匹配的 pass 事件；PS3 audit 在 terminal_attempt 子场景处被总期限中断，剩余项拆组补验，不提高原期限。历史证据目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T102815.418606700Z/`。

4. 首次并行补验未显式指定输出目录，三个 runner 的默认时间目录重名，`20261013T103948.207613300Z` 中的日志/报告混写而无效，不计为通过；随后以各组独立输出目录重跑，保持原场景与原超时上限；报告完成后原样移至系统临时目录的独立 UUID 目录，原报告内容不改写。

5. PS3 audit 独立重跑：passed/complete，7 项、0 失败/跳过，100.845 秒；证据原输出目录为工作区 `$candidateAuditOut`，已原样移至 `C:/Users/lmas1/AppData/Local/Temp/steerline-candidate-47ac4d29-7646-450f-b7e6-f303538b38bc/summary.json`。
6. PS3 资源与停止回执独立重跑：passed/complete，4 项、0 失败/跳过，83.984 秒；证据原输出目录为工作区 `$candidateResourceOut`，已原样移至 `C:/Users/lmas1/AppData/Local/Temp/steerline-candidate-24be6369-33a1-44ff-899f-9a1f8ae72e41/summary.json`。
7. TaskStartControl 独立重跑：passed/complete，11 项、0 失败/跳过，357.236 秒；证据原输出目录为工作区 `$candidateControlOut`，已原样移至 `C:/Users/lmas1/AppData/Local/Temp/steerline-candidate-control-1ba82018-8787-40f2-9fce-53e0f3d16820/summary.json`。

本次未做真实终端人工验收，不以单元渲染断言声明真机窗口验收通过。安装包更新不属于本次操作。

## 2026-10-13 任务提示与候选概览补充

用户要求任务提示由多个状态综合计算，支持“等待合并”，候选与执行窗口隐藏 execution_status/workflow_status。任务提示按同 Task 的所有当前候选计算；Task 终态优先，正常等待合并候选优先于其他候选的失败/阻塞，切换候选不改变任务提示；PR 查询失败或恢复未核实时不能冒充等待合并。右上仅隐藏两个字段，原始完整字段仍可在 d 详情读取。

最小验证：`TestTaskHintCombinesAllCandidates` 覆盖正常/异常轴、两个不同候选的聚合与选择稳定性，既有窄/宽屏及详情测试更新为新主界面要求；只运行 console 包，不涉及执行引擎或 PR 交付判定，无需 L2/L3/Full。首次 console 回归仅旧 `TestUX1ParallelNumbersAndPrimarySurface` 仍断言主界面显示两个已移除字段而失败；更新为新可见/隐藏字段要求后重跑，最终结果另登记。原失败报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T105533.225696400Z/summary.json`。

最终重跑：passed/complete，198 项、0 失败/跳过，4.067 秒；证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T105744.351369400Z/summary.json`。本次未修改 PR 关闭或关闭后轮询业务规则。

## 2026-10-14 PR 集成验证

提交 PR 前合入最新 `origin/main`（`ce1d2c5a`），清除新 TUI HTTP 客户端中的已删除候选放弃 action/专用 payload 引用。首轮集成编译发现该残留，未登记通过；清除后重跑 `go run ./cmd/test-feedback -out <独立临时目录> -- ./internal/console ./internal/app ./internal/consoleapp ./internal/product/api ./internal/product/httpapi ./internal/product/httpclient ./internal/product/query ./cmd/steerline -count=1`，最终 passed/complete，225.837 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-pr-candidate-http-40d782b2aaa848/summary.json`。仍使用隔离夹具，不包含安装环境或真机窗口验收。
