# Runner 执行展示与最近消息验收

[Plan / 测试映射](../plans/execution-observability.plan.md)。普通开发；Windows / Go 1.26.4 / Node，2026-10-15。

| 项 | 要求 | 状态 |
| --- | --- | --- |
| EOV-A01 | 流程/状态可读，阻塞诊断保留，技术噪声不挤掉五条变化，固定身份 | 通过；定向 SQLite 及噪声子场景最终复验通过 |
| EOV-A02 | 新 HTTP 路由和 CLI 接通，错目标零查询、观察时间不被轮询改变 | 通过；HTTP/统一路由与 runnerclient 回归通过 |
| EOV-A03 | 仅当前可见执行轮询；切换/后台/卸载取消、旧响应隔离；错误保留样本 | 通过；Vitest 三文件最终 22 项通过，0 失败/跳过 |
| EOV-A04 | Web 类型/生产构建与三 EXE 编译 | 通过；Web 类型/生产构建及两仓三 EXE 编译通过 |
| EOV-A05 | Runner 公开契约受控真实 EXE 验证，查询零副作用 | 通过；Runner ROV-A03 实际 EXE/服务受控闭环通过 |
| EOV-M01 | 用户检查真实详情可读性、长消息与切换体验 | 待人工，允许后置，不阻塞 PR |

环境与限制：Go 用隔离临时目录/SQLite/本地 mock 协议，前端用 jsdom；没有使用或修改 `C:\SteerlineInstall`、`C:\SteerlineRuntime`，没有重跑任务 105，没有真实模型/GitHub 业务 PR 或 Orca 验收。Runner 仅进程内保存一条摘要，服务重启后暂无样本，旧 event 缺少 transition 不补画。两仓需一起构建发布，详情页才能调用新增 CLI。

## 最终自动结果

2026-10-15 18:20（北京时间）完成最小定向复验：Plan 中 EOV-A01/A02 的 Go 命令通过（含子用例共 15 个 pass，0 fail/skip；额外 app/client 及全部 product 子包完成编译）；最后自查同时固定 candidate StateDir/Session 并使用配置副本读取，随后 Go 定向复验与编译再次通过。Web 三文件 22 项通过；`npm run build` 通过。三 EXE 输出仅写入两仓 `.evidence/observability/`，Web 生产资源构建后重编 `steerline-web.exe`。`git diff --check` 通过。

证据：本仓 `.evidence/observability/go-final.jsonl`、`web-final.json`；Runner `.evidence/observability/integration/integration.log`、`result.json` 与 `caller-commands.jsonl`。受控真实 EXE 的 `TestSRRunWorkflow` 1 项通过（场景 20.04 秒，含构建总计 29.428 秒），包括公开 CLI last-message 两次读取的原时间一致、零 revision/native turn 增量，以及 workflow_changed 真实历史与后续原会话反馈。原始证据目录忽略入库，本文保留可审查摘要。

人工项仍待验；上述通过不代表真实原生权限、认证、恢复或 GitHub 业务交付重新验收。两仓 PR 合并核验在提 PR 后登记。

跨仓依赖：[SteerRunner PR #6](https://github.com/erranliu/SteerRunner/pull/6)，实现提交 `83d902d`。先合并 Runner，再合并本仓并用两仓新源码构建三 EXE；安装配置继续按新 EXE SHA 固定，不使用旧 Runner 回退。PR 合并与人工体验验收尚未完成，不以已提 PR 声称已上线。
