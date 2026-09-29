# 本地 app-server 服务级补验（2026-09-29）

环境：Windows、Codex CLI `0.157.1`，使用独立的临时 Git 仓库、SQLite 状态和 worktree。Codex 模型为 `gpt-6-luna`、推理强度 `low`、只读 sandbox。当前 Codex 桌面宿主位于不允许子进程脱离的 Job Object；通过同一用户的 WMI `Win32_Process.Create` 启动临时测试宿主，诊断确认该宿主 `child_in_job=false`。临时计划任务仍位于 Job Object，未用于最终验收。所有真实 Codex 子进程按保存的进程身份清理。

| 验证 | 结果 | 关键证据 |
| --- | --- | --- |
| `TestAppServerServiceParallelRecoveryLive`，真实 Codex 双任务 | 通过，361.87 秒 | 同一 SteerLine Service 中第一任务的 `steerline_ask` 待答时，第二个 Execution 在独立 app-server/thread/worktree 完成。关闭观察并重建服务对象后，原 call ID、thread、turn、工作区不变；统一 Decision 答案回复原请求，第一轮完成。结束后反馈在原 thread/worktree 上创建并完成不同的第二个 turn。 |
| `TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace`，独立宿主模拟进程 | 三例通过，23.31 秒 | 自由文字、选项加备注、开放 PR 后反馈均在原会话和工作区续轮；重建 Service 后保留原 Execution/Attempt，部分提交的 HEAD 不被重置，未知发送不盲重试。 |
| `TestAppServerHostExitRecoveryLive`，真实 Codex 宿主进程退出 | 通过，125.76 秒 | 子进程在原生问题待答且 SQLite 回执已保存时直接 `os.Exit(0)`；新进程确认原 app-server PID 仍为已保存的身份，重接原 thread/turn/call，提交答案并完成原 turn。 |

双任务真机记录的第一任务 thread 为 `01a0ed66-e0f8-73f3-bcfd-9eb17cb8c6dd`，两轮 turn 为 `01a0ed66-e9e0-7602-9984-db06f1bc317b` 和 `01a0ed6a-890d-7a42-8299-4ec0180d088e`；第二任务 thread 为 `01a0ed68-b606-7123-8064-cac7d67fed9b`、turn 为 `01a0ed68-b882-7451-811c-5a5c1417dac2`。宿主进程退出补验的 app-server PID 为 `9088`，thread 为 `01a0ed70-35c6-77e1-9d6f-37a3960f7f27`，turn 为 `01a0ed70-3dd7-7bd2-a43f-29cb6149b214`，原问题 call ID 为 `exec-e0a94c8b-db06-4224-a766-425a6d3f9d4f`。

补验发现并修复两处问题：Windows 上同一 worktree 路径的大小写在首次保存与重启加载后可能不同，导致不可变执行绑定误报冲突；`saveBinding` 现按平台路径规范比较，仍严格核对其他身份字段。宿主发布说明追加到反馈提示后，原服务测试的“纯反馈文本”断言已调整为完整提示。新增 Windows 路径大小写回归，服务反馈三例与真实服务级用例均在修复后通过。

真实 Codex 连接期间曾出现 `wss://chatgpt.com/backend-api/codex/responses` 的短暂重连；最终三个真机 turn 均完成，未以中间重连事件判定通过。协议级旧探针见 [probe](probe-2026-09-29.md)。Go race detector 因当前环境没有 CGO/C 编译器而未执行，保留在 [Roadmap 第一阶段残留](../../roadmap/roadmap.md#第一阶段残留)。
