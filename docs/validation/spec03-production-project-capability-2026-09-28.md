# SteerLine 项目的 home 受监管能力认证（2026-09-28）

按生产目标为 `erranliu/SteerLine` 的假设，在 home Orca 1.4.210 新增该仓库的独立 clone `C:\project\SteerLine`，仓库 ID `c67f8c64-d472-449b-82e3-24df822d0fdb`。本机中心 Orca 为 1.4.209，远端 Codex CLI 为 0.157.1。只读工作树 `prod-capability-20260928` 固定在 `1f6a9ce4c104df6b9638e7587c74bc85323aa971`；[项目/仓库](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/home-repo.json)、[工作树](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/home-worktree.json)与主机侧[Git 原始输出](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/host-git-read.json)确认 origin、分支、HEAD 和干净状态。没有编辑、推送或创建 PR。

固定 Orca Run `run_707f6533c393` 的首次试次复用事先启动的终端 `term_b5bf8ac3-ddb5-4e0f-a162-cf62ec3f0921`。它能发送绑定 status，却无法结算 `worker_done`：无显式地址返回“找不到接收方/活跃 Dispatch”，显式 Run 地址又被联邦路由拒绝；worker 报告两次均带原 `--from` 与 `--dispatch-capability`。此试次没有被误记为成功；操作者仅对精确 Dispatch `ctx_5dfac74462a6` 执行一次 `worker-stop`，回执 `closed_agent_terminal/ptyKilled=true`，Task 留 blocked、原终端由执行主机确认为 exited。[失败试次 fleet](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/first-worker-list.json)与[原消息](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/first-mail.json)保留。**复用预启动终端的 worker_done 路径未通过。**

随后按 Orca 的固定 `--retry-of ctx_5dfac74462a6` 在同一 Task/工作树上由 `worker-start --agent codex` 新建 agent 终端，产生 Dispatch `ctx_3ceb7b58383c`。这是当前 `orca_supervised` 适配器使用的终端所有权路径。该试次发出精确 status 与唯一 `worker_done`，Task `task_ae1a7cb84662` 完成；`worker-release` 原请求 `8cab3e4d-7da4-4dc4-aee6-1bd5337fb9a5` 已确认 `released/closed_agent_terminal` 并归档 Codex transcript。最终[Task](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/final-task-list.json)、[两试次 fleet](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/final-worker-list.json)、[消息收件箱](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/final-mail.json)、[release](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/release-summary.json)与[零存活终端](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/final-terminal-list.json)分别核对。

| 当前 SteerLine/home 目标所需能力 | 判定依据 |
| --- | --- |
| `start` | 独立项目工作树与原 Task 的新 agent 终端 `worker-start` 返回 `ready`，后续同一 Dispatch 完成。 |
| `observe` | `worker-show` 与 fleet 按 Run/Task/Dispatch、home execution host、终端 handle/incarnation 对上，结算后观察为 exited。 |
| `workspace_inspection` | 项目 origin、固定工作树和独立主机 Git 输出确认分支、HEAD 与无改动。 |
| `output_reading` | `worker-read --source auto` 在 release 后仍取到已归档、`sourceExact=true` 的 Codex [transcript 回执](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/worker-read-summary.json)；剪裁字段保留，未将其当作完整正文。 |
| `message_inbox` | status 与 worker_done 均以原 Task/Dispatch 身份进入中心 FIFO Delivery，逐条处理并 ack。 |
| `resource_settlement` | 原 release 请求已完成，归档 transcript，Owned agent 终端 exited；工作树无存活测试终端。 |

上述六项的[内容哈希清单](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/capability-manifest.json)和[当前格式能力报告](../spec/orca-probe/samples/spec03-prod-cap-2026-09-28/capability-report.json)已在独立状态目录保存、重读并重新核验每个引用文件的 SHA-256。`remote_cancellation` 与 `stop` 仍为 `unsupported`，精确 `output_turns`/`output_final_answer` 仍为 `not_tested`；本报告不把复用终端的失败覆盖成成功。

这是 **SteerLine 仓库的 Orca 项目能力认证**，不是 SteerLine 自身原生 Task 的五步生产演练，也没有部署生产配置。工作树创建时的 HEAD 为 `1f6a9ce`，随后本机独立 clone 读到更新的 main `1b2aedc`；后续原生演练须重新固定当时的 base/head，不外推本次只读工作树为最新代码。能力报告的运行时副本位于隔离状态目录，正式服务配置须在其自己的状态目录重新绑定证据。Windows 管理员/UAC 可见捕获、完整故障矩阵及上述未测的精确轮次能力继续开放。
