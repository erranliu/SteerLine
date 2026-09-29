# 本地 app-server 迁移验收状态（2026-09-29）

实现已接入 `worker-flow-v1` 本地派发，协议为 `local_codex_app_server_v1`。状态仍为实施中；以下区分真实 Codex、模拟回归和未完成的交付门槛。

| Spec 验收组 | 当前证据 | 状态 |
| --- | --- | --- |
| A 双任务真机闭环 | [Windows 协议探针](probe-2026-09-29.md)使用真实 Codex，验证两独立 app-server 并行、第一条待答时第二条完成、断线重接、不同连接回复原请求、匹配的工具结果，以及结束后反馈在原 thread 开启并完成新 turn。 | 部分通过。当前测试宿主的 Job Object 阻止 SteerLine worker 脱离服务生命周期；尚未验证 SteerLine 服务级双任务与服务重启完整闭环。 |
| B 针对性自动测试 | 原生问题与多题答案、请求回放、匹配答案证据、WebSocket 帧边界、旧协议拒绝及 mock app-server thread/turn 契约通过。[审计修复](audit-2026-09-29.md)补充第二轮回执丢失、重复启动通知、SQLite 答案与队列持久化、慢连接隔离、取消及终态迟到答复回归。 | 部分通过。依赖持久独立子进程的服务反馈用例在当前宿主跳过；仍需在可脱离 Job Object 的宿主验证服务级反馈、宿主退出与完整双任务隔离。 |
| C 既有契约回归 | `go test ./... -count=1` 全量通过；旧本地协议拒绝测试证明拒绝前未准备工作区、未改写 Execution。`go vet ./...`、Windows 与 Linux 构建通过。 | 自动回归通过；按 Spec 未改交付逻辑，无需重做人工真实合并。 |

当前真机 Codex CLI 为 `0.157.1`，app-server `initialize.userAgent` 为 `Codex Desktop/0.158.0-alpha.2.1`。npm 包装器与原生可执行文件版本核对已通过。真实探针验证了协议能力，尚不能替代服务级 A/B 门槛。
