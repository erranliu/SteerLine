# 最小闭环与日常快速入口实验

源码基线 `543f0d4e9dd9559f253de663d3ffb646ceecf079`，Windows/amd64、Go 1.26.4，已有 Go 缓存。真实平台/可见窗口验收开关未启用。测试用临时 Go overlay 新增一个用例；工作区产品 Go 文件未修改，任务及 DAG 未改变。[完整测试提速记录](../README.md)中的 658.633/264.871 秒对照来自另一轮实验，这里的快速命令独立计时。

## 用例内容

`TestNativeMinimalClosedLoopSmoke` 从[原本机闭环](../../../../internal/execution/native_tasks_test.go)取必要初始化，创建 A → B；本机进程测试 worker 在真实临时 Git 仓库和 SQLite 中完成 A 的 implement、verify、publish_pr，人工合并门槛前 B 仍阻塞。测试重启中心并核对原 WorkflowRun/PR 等待状态，随后通过隔离的测试 PR 提供者返回固定 head 合并回执，中心关闭 A、写入唯一 TaskDelivery、解锁 B。B 不再执行；流程配置 `review_required=false`，完整审查、失败重试、Decision、清理及受监管 Orca 独有场景仍在原测试中。

这个用例实际启动本机测试 worker，并调用原中心查询/命令与恢复代码。PR 回执来自测试提供者，不能证明真实 GitHub/Gitea 合并。另保留原 `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce` 的错误 PR/交付门槛检查，再运行 taskgraph 和 remoteprotocol 的完整纯规则测试。

## 运行与结果

本轮对照用 Go overlay 编译临时核心测试程序，准备/编译时间单列；快速命令本身依次执行两部分：

下列是实现后的日常命令形状。本轮首项实际使用 [manifest](manifest.json) 指向的临时 overlay，并核对两个顶层用例各有一次通过终态。当前仓库尚无该 smoke 用例，直接运行没有 overlay 的第一条命令不能作为通过证据。

```text
go test -count=1 -timeout=2m ./internal/steerline -run '^(TestNativeMinimalClosedLoopSmoke|TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce)$'
go test -count=1 -timeout=2m ./internal/steerline/taskgraph ./internal/steerline/remoteprotocol
```

| 检查 | 实际墙钟 | 结果 |
| --- | ---: | --- |
| 闭环 + 错误 PR 门槛 | 37.652 秒 | 两项通过；闭环本身 29.25 秒、门槛 6.72 秒 |
| 两个纯规则包 | 1.518 秒 | 全部通过 |
| 合计 | **39.170 秒** | 零失败、零跳过 |

[实验用例](smoke-test.go.txt) · [overlay 身份](manifest.json) · [快速结果](quick-summary.json) · [核心测试原始输出](quick-0.jsonl) · [纯规则原始输出](quick-1.jsonl)。首个单独闭环运行另见 [smoke 原始输出](go-test.jsonl) 和 [计时](summary.json)。

## 采用边界

快速结果只标为 `quick`，适用于日常编辑后与受影响的定向测试一起使用。稳定候选集成或实现 PR 交付前仍运行一次完整无缓存套件；既有任务已固定的完整/真机要求保持。快速套件不会据单次成功推断失败重试、未知外部结果、Orca 或真实 PR 合并已通过。[Spec 草案](../../../archive/spec/completed/test-execution-efficiency.spec.md)固定实现与门槛，尚未创建中心任务。
