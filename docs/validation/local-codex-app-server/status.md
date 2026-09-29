# 本地 app-server 迁移验收状态（2026-09-29）

实现已接入 `worker-flow-v1` 本地派发，协议为 `local_codex_app_server_v1`。按用户确认，实现 PR 可合并；未完成验收转入 [Spec](../../spec/local-codex-app-server.spec.md#第一阶段残留2026-09-29) 和 [Roadmap 第一阶段残留](../../roadmap/roadmap.md#第一阶段残留)，以下保持真实、模拟及未测证据的区分。

| Spec 验收组 | 当前证据 | 状态 |
| --- | --- | --- |
| A 双任务真机闭环 | [Windows 协议探针](probe-2026-09-29.md)使用真实 Codex，验证两独立 app-server 并行、第一条待答时第二条完成、断线重接、不同连接回复原请求、匹配的工具结果，以及结束后反馈在原 thread 开启并完成新 turn。 | 部分通过。当前测试宿主的 Job Object 阻止 SteerLine worker 脱离服务生命周期；尚未验证 SteerLine 服务级双任务与服务重启完整闭环。 |
| B 针对性自动测试 | 原生问题与多题答案、请求回放、匹配答案证据、WebSocket 帧边界、旧协议拒绝及 mock app-server thread/turn 契约通过。[审计修复](audit-2026-09-29.md)补充第二轮回执丢失、重复启动通知、SQLite 答案与队列持久化、慢连接隔离、取消及终态迟到答复回归。 | 部分通过。依赖持久独立子进程的服务反馈用例在当前宿主跳过；仍需在可脱离 Job Object 的宿主验证服务级反馈、宿主退出与完整双任务隔离。 |
| C 既有契约回归 | `go test ./... -count=1` 全量通过；旧本地协议拒绝测试证明拒绝前未准备工作区、未改写 Execution。`go vet ./...`、Windows 与 Linux 构建通过。 | 自动回归通过；按 Spec 未改交付逻辑，无需重做人工真实合并。 |

当前真机 Codex CLI 为 `0.157.1`，app-server `initialize.userAgent` 为 `Codex Desktop/0.158.0-alpha.2.1`。npm 包装器与原生可执行文件版本核对已通过。真实探针验证了协议能力，尚不能替代服务级 A/B 门槛。

同步主线 `f026fe6` 后，改用当前 worker-flow 接口并适配宿主 PR 发布的原生 turn 身份；修正最小测试入口引用的已删除测试。最小入口加 app-server/worker-flow/宿主发布定向回归通过（61.1 秒，无跳过），`cmd/test-feedback` 测试、全部包编译、`go vet ./...`、Windows/Linux 构建通过。全量测试证据来自同步前；同步后未重复全量。定向选择排除了需特殊宿主的服务反馈用例及另行启用的 Orca 真机用例，前者仍列在 B 残留。
