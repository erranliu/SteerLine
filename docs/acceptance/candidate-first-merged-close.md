# 多候选任务首个核验合并后关闭

2026-10-14 用户直接授权的普通开发：任一候选 PR 核验已合并，将 Task 转为 `closed`；关闭后停止该任务所有候选的 PR 状态监控。未修改 roadmap 或安装环境。

## 实现边界

- 交付只查询触发交付候选当前已绑定的 PR，仍核验冻结提供方、仓库、base/head 分支身份和完整合并证据。未绑定的发布线索不提前绕过发布阶段。
- 其他候选已合并、查询失败或继续推进步骤都不阻塞该交付；获胜候选自己的绑定、任务/图版本、重复 PR 绑定仍在事务中复核。`expected_set_version` 在交付声明中绑定获胜候选自身，不再绑定所有兄弟的阶段状态。
- PR 合并不等待该候选追加反馈结束。开放 PR 查询保留原输入/当前步骤；已核验合并不派发新输入，直接交付。
- 唯一 TaskDelivery、closed、规范结果与后继释放保持原子提交。首个成功提交的记录成为获胜结果；不按所有 PR 的合并时间排序，后续请求不覆盖原交付。
- 主 PR 轮询和单候选轮询都在 closed 时返回，移除退休候选 PR 查询与关闭后的迟到合并事件采集。重启保持跳过。界面保留最后已知 PR 事实，多个候选已合并不显示为交付冲突。
- Task 丢弃仍使用完整候选 PR 未合并核验，丢弃后的合并核实规则保留。工作区、分支、PR 和外部执行的物理状态不由关闭后的 PR 轮询推断。

## 自动验收

| 项 | 通过条件 | 场景与结果 |
| --- | --- | --- |
| FMC-A1 任一候选合并即可关闭 | 多个 PR 已合并或兄弟查询不可用时，只查询选定 PR 一次并 closed；兄弟在查询和提交之间推进也不阻塞 | `TestMC6/multiple_merged`、`unknown_sibling`，passed |
| FMC-A2 closed 后不监控 | 同一轮闭合后全任务/单候选轮询均为 0 次查询，PR 缓存不变；重启后仍 0 次查询，原交付不变 | `TestMC6` 三子场景，含 `late_after_delivery`，passed |
| FMC-A3 不等待追加反馈 | 开放 PR 不改原待投递输入或实现步骤；绑定 PR 合并可在反馈启动中 closed，不增加输入/轮次数 | `TestCandidateOpenPRPollPreservesOriginalPendingInput`，最终 passed；`TestCandidateClosedPRPollDeliversLaterMerge`，passed |
| FMC-A4 原子交付和身份守卫 | 事务失败不部分关闭/释放；原请求重试仍一个结果；错误 PR 身份、未合并、无选定 worker 拒绝；Task 丢弃仍核验兄弟 PR | `TestMC5`、`TestMC7`、`TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged`、`TestDeliveryGateRequiresSelectedWorker`、SQLite 包，passed |
| FMC-A5 展示与重建 | 不再把多个合并 PR/落选候选合并显示为交付冲突，原始 PR 和资源事实保留；重建结果与只读查询可用 | console、model、reconstruction 包，passed |

## 命令与证据

Windows/amd64，既有隔离 Go 夹具，使用本地测试 Git/provider/worker helper，无真实模型和真实 PR；未使用 `C:\SteerlineRuntime`。只选择受影响包和候选交付定向场景，不跑 Full/L2/L3。

1. `go run ./cmd/test-feedback -out <独立临时目录> -- ./internal/console ./internal/execution/model ./internal/execution/reconstruction ./internal/storage/sqlite -count=1`：passed/complete，274 项、0 失败/跳过，28.889 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-first-merge-packages-68005219272fa/summary.json`。
2. `go run ./cmd/test-feedback -out <独立临时目录> -- ./internal/execution -run '^(TestMC5|TestMC6)$' -count=1`：passed/complete，5 项、0 失败/跳过，133.630 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-first-merge-delivery-bcb5007844c26/summary.json`。
3. 其余定向组的报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-first-merge-pending-df536e2fa0762/summary.json` 为 failed/complete、10 项中 1 项失败，不能登记整组通过。关闭未合并 PR 后真实合并、Task 丢弃、worker 身份和 provider 身份守卫测试均有匹配的 pass 事件；失败的追加反馈夹具直接替换原 turn ID，被既有身份不可替换守卫正确拒绝，发生在 PR 轮询前。
4. 将该夹具改为保留原 turn 身份、原待投递输入，只改变反馈启动状态/步骤后，执行 `go run ./cmd/test-feedback -out <独立临时目录> -- ./internal/execution -run '^TestCandidateOpenPRPollPreservesOriginalPendingInput$' -count=1`：passed/complete，24.517 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-first-merge-feedback-3f4fed6dd3316/summary.json`。未修改生产代码或放宽原 turn 身份校验。

无任务人工项；本次未做真实提供方/真实窗口验收，不把隔离夹具结果声明为真机验收。
