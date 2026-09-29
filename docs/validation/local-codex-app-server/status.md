# 本地 app-server 迁移验收状态（2026-09-29）

实现已接入 `worker-flow-v1` 本地派发，协议为 `local_codex_app_server_v1`。A、B 服务级验收现已补齐；Go race detector 仍列在 [Spec](../../spec/local-codex-app-server.spec.md#第一阶段残留2026-09-29) 和 [Roadmap 第一阶段残留](../../roadmap/roadmap.md#第一阶段残留)。以下区分真实 Codex、模拟回归和环境限制。

| Spec 验收组 | 当前证据 | 状态 |
| --- | --- | --- |
| A 双任务真机闭环 | 真实 Codex 的 `TestAppServerServiceParallelRecoveryLive` 在同一 SteerLine Service 中领取两个 Execution：第一条原生问题待答时第二条完成；关闭并重建服务对象后，原 call ID、thread、turn 与 worktree 均恢复，答案送达原请求，结束后反馈在同一 thread/worktree 开启不同的第二轮。进程及会话身份记录见[补验记录](acceptance-2026-09-29.md)。 | 通过（真实服务级，361.87 秒）。 |
| B 针对性自动测试 | 原生问题、多题答案、请求回放、匹配答案证据、WebSocket 帧边界、旧协议拒绝及[审计修复](audit-2026-09-29.md)回归通过。在独立宿主，`TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace` 的自由文字、选项＋备注、开放 PR 后反馈三例全部通过；`TestAppServerHostExitRecoveryLive` 让承载 SteerLine 的子进程在原生问题待答时真正退出，新进程确认 app-server 存活、重接原身份、回复并完成原 turn。 | 通过（定向自动测试与真实宿主退出，分别 23.31 秒和 125.76 秒）。 |
| C 既有契约回归 | `go test ./... -count=1` 全量通过；旧本地协议拒绝测试证明拒绝前未准备工作区、未改写 Execution。`go vet ./...`、Windows 与 Linux 构建通过。 | 自动回归通过；按 Spec 未改交付逻辑，无需重做人工真实合并。 |

当前真机 Codex CLI 为 `0.157.1`，app-server `initialize.userAgent` 为 `Codex Desktop/0.158.0-alpha.2.1`。npm 包装器与原生可执行文件版本核对已通过。先前的[协议探针](probe-2026-09-29.md)现由服务级 A/B 证据补足。

同步主线 `f026fe6` 后，改用当前 worker-flow 接口并适配宿主 PR 发布的原生 turn 身份；修正最小测试入口引用的已删除测试。最终补验后的最小入口加 app-server/worker-flow/宿主发布定向回归通过（58.7 秒，无跳过），`cmd/test-feedback` 测试、`go vet ./...`、Windows/Linux 构建通过。全量测试证据来自同步前；同步后未重复全量。独立宿主反馈和真机服务测试按可选入口另行运行，未计入最小入口。
