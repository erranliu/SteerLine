# 任务启动与新增候选资格

2026-11-12，源码基线 `3b99ee3172694ed7f020443eb2127e7d8b131247`，验证包含本次未提交改动。在线任务详情按 [本机客户端协议](../../../docs/product-design/local-client-protocol.md) 返回动作、任务阻断和执行方案阻断。Web 消费后端结果并绑定原任务、图及派发版本。首次请求尚未确认时，排队的确认等待原领取回执并重新读取新增候选资格。

候选上限按当前材料摘要统计。读取预览和预留事务共用计数查询，事务保留依赖、任务版本、材料和环境证据复核。已有候选的公开 PR 状态为 merged 时阻断新增候选，任务交付仍由独立的平台核验和交付事务确认。自动派发模式沿用后端行为，允许对进行中的任务显式新增合格候选。

## 验证

| 范围 | 结果与证据 |
| --- | --- |
| L1 `go run ./cmd/test-feedback -level=L1` | 通过。报告为 `%TEMP%/steerline-start-L1-b1d6627bd58543adbf693ddbd61d65e2/summary.json`。 |
| L2 `go run ./cmd/test-feedback -level=L2` | 通过。报告为 `%TEMP%/steerline-start-L2-ee45286a23494ac4a169af4b9d00c262/summary.json`。 |
| HTTP 专项 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http` | 通过。报告为 `%TEMP%/steerline-start-http-e653aa7c6a6c4f7793a5e917c55e1b51/summary.json`。 |
| SQLite 专项 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.storage` | 通过。报告为 `%TEMP%/steerline-start-storage-72695ce962a24086ab3ce113b6ee978b/summary.json`。 |
| Go 定向资格及预留 | `go test ./internal/app ./internal/storage/sqlite -run 'Test(TaskStartEligibility\|TaskStartPreviewAndReservationUseCurrentMaterialLimit\|ReadinessAdmissionPolicyReservation)$' -count=1` 通过。包含跨材料计数，以及预览后其他请求占满名额时拒绝并保持候选数。 |
| Go 任务与路由 | `go test ./internal/taskrunner ./internal/product/applicationhttp -count=1` 通过。 |
| Web 全集 | `npm --prefix web test -- --maxWorkers=2` 通过，53 个文件、488 个用例。默认并发首轮有两个查询等待超时，降低并发后完整复跑通过。 |
| Web 生产构建 | `npm --prefix web run build` 通过。 |

本次覆盖受控交付、组件交互及本地构建。真实模型、安装部署和浏览器现场体验未验证。
