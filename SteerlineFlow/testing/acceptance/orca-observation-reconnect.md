# Orca 观察重连展示验收

受测基线 `32b452bc` 加本次 worktree 改动。候选使用 Runner 已有的公开 phase。自动重连保持在「运行中」筛选，候选标记「正在重连」。恢复请求计入「需处理」，用「继续原任务」提交绑定原候选与 Attempt 的操作。真正的问题仍进入「待回答」。

| 验证 | 结果 |
| --- | --- |
| `npm --prefix web run test -- src/features/runs/state.test.ts src/features/runs/runs.test.tsx src/features/runs/answer.test.tsx src/features/runs/answer-suggestion.test.tsx` | 4 个文件、54 项通过，含重连分类、恢复入口单次提交及原 Attempt 绑定 |
| `npm --prefix web run build` | TypeScript 与生产构建通过 |
| `go test ./internal/taskrunner ./internal/product/query` | 两包通过 |
| `go run ./cmd/test-feedback -level=L1` | 通过，报告 `C:/SteerlineDebug/wt-beae-observation-reconnect/test-results/L1/reconnect-01/summary.json` |
| `go run ./cmd/test-feedback -level=L2` | 通过，报告 `C:/SteerlineDebug/wt-beae-observation-reconnect/test-results/L2/reconnect-01/summary.json` |

首次沙箱内 npm 安装和部分 Go 用例因本机临时目录写权限失败，以上为补验结果。未在安装现场部署，也未操作真实候选。

## 恢复回执增量

应用按产品请求 ID 对账 Runner 恢复动作，保留各自独立的请求 ID。原请求尚在核验时保持防重。最终失败显示对应的 Runner 诊断，用户可以按当前版本向同一 Attempt 发出新的恢复请求。新结果取代候选摘要中的旧恢复失败提示。

| 验证 | 结果 |
| --- | --- |
| `go test ./internal/app -run TestRunnerResumeReceipt -count=1` | 通过，覆盖原请求绑定、待核验、最终失败及成功、较新的诊断不归给旧请求 |
| `go test ./internal/taskrunner ./internal/product/query ./internal/product/httpapi -count=1` | 通过，HTTP 回归核对原请求最终失败及原因 |
| `npm --prefix web run test -- src/features/runs/answer.test.tsx src/features/runs/answer-suggestion.test.tsx src/features/runs/runs.test.tsx src/features/runs/presentation.test.ts src/features/runs/state.test.ts` | 5 个文件、58 项通过，覆盖最终失败、刷新后重试、当前版本和防重复提交 |
| `npm --prefix web run build` | TypeScript 与生产构建通过 |
| `go run ./cmd/test-feedback -level=L1` | 通过，报告 `C:/SteerlineDebug/wt-beae-observation-reconnect/test-results/L1/resume-receipt-01/summary.json` |
| `go run ./cmd/test-feedback -level=L2` | 通过，报告 `C:/SteerlineDebug/wt-beae-observation-reconnect/test-results/L2/resume-receipt-01/summary.json` |

本轮仅用隔离夹具验证，未部署或操作 Task 17。
