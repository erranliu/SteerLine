# 本机 Codex app-server 验收记录

唯一归属：[本机 Codex app-server 执行迁移 Spec](../../spec/local-codex-app-server.spec.md)，配套 [Plan](../../plans/local-codex-app-server.plan.md)。这是原任务的专属记录，不新建 Task ID。本次只拆分既有记录，没有执行补验或改变结果。

## 已验范围

2026-09-29 的 A/B/C 已通过：同服务双任务、原 thread/turn/call 恢复、原会话第二轮；自由文字、选项加备注、开放 PR 反馈；真实宿主退出后的原身份重接；自动回归、vet 与 Windows/Linux 构建。原 Codex CLI 0.157.1、app-server userAgent Codex Desktop/0.158.0-alpha.2.1。同步主线后用 58.7 秒定向回归补验，没有重复全量。

独立 app-server 存活属于旧宿主语义，不能替代当前受限 Job 下的人工继续恢复。精确方法、版本、时长与原证据见[原状态](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/local-codex-app-server/status.md)；不把历史通过扩展到新宿主或构建。

## 待验项

| 验收项 | 方法 | 状态 / 所需安排 | 通过条件与阻塞范围 |
| --- | --- | --- | --- |
| AS-RACE-A：app-server 定向 race | 自动 | 待验收；待 CGO/C 工具链，按 Plan 的并行/恢复/回复定向命令执行 | 选定场景真实命中并通过 race；结论未取得，不改称人工补验 |

实际命令、构建和结果在准备时补入本页，原始输出留临时目录；需人准备环境时单独约窗口，不改变自动项通过条件。当前未新增人工验收项。验收流程见[入口](README.md)，execution-map 同步未验链接和结果汇总。
