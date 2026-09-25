# 状态模型：SteerLine 三个轴与 Beads 的关系

同一任务在 SteerLine 和 Beads 里各有一份"状态"，容易被误读成"两套矛盾的状态"。它们其实是**不同轴**：一个是进程/交付生命周期，一个是任务认领工作流。本文固化这套模型，并说明 Beads 的定位（过渡性任务提供方）。原生 Task 模型已经加入同一个中心 SQLite；本阶段只允许 native 权威初始化和只读查询，生产领取/执行仍由后续 Beads 退出任务接入。

## 三个正交轴

| 轴 | 字段 | 取值 | 权威 | 回答的问题 |
| --- | --- | --- | --- | --- |
| 执行 | `execution_status` | pending / running / succeeded / failed / unknown | SteerLine | agent 进程/终端在跑吗？正常退出吗？ |
| 交付 | `finish_task_status` | pending / waiting_decision / waiting_pr_merge / completed / blocked | SteerLine | 这次运行在等决策、等合并，还是已完成/阻塞？ |
| 任务/认领 | 当前 `beads_status`（镜像），将来 SteerLine 任务态 | open / in_progress / closed | 外部任务提供方（当前 Beads） | 任务被认领了吗？谁认领？关闭了吗？ |

另有独立的 `pr.state`（open / merged / closed_unmerged / unknown，平台权威）与 `decision.status`（pending / answer_saved / resuming / reply_accepted / applied / failed / superseded，见 [decisions](decisions.md)）。

服务还有独立的控制轴 `control.dispatch_mode`（manual/auto）、`dispatch_paused` 和 `revision`。它们控制将来是否领取新任务，不改变执行、交付或 Beads 认领事实。`control.operation` 和 `control.blockers` 是控制协议的持久检查点/不确定副作用保留，不是任务队列；服务是 runtime 的唯一写者，客户端通过文件协议或 SQLite `CommandService` 提交当前实例绑定的命令。

PR 4A 的 `ExecutionRef` 和后端 `Operation` 纳入同一个 `steerline.db`：ExecutionRef 固定 backend/protocol/target/agent/workspace 和外部身份，Operation 固定动作负载摘要及 pending/in_flight/accepted/confirmed/rejected/outcome_unknown 结果。运行身份在 runtime v5 的 `Run` 投影中，数据库绑定必须与它的 PID/创建身份或 Orca runtime/terminal/incarnation 一致；未知、损坏或冲突记录阻止有副作用的恢复。配置摘要改变不会重路由历史记录。`execution/v1/` 仅保存能力报告和原生证据，不作为执行状态恢复源。

**关键**：`execution_status=running`（进程在跑）与 Beads `IN_PROGRESS`（任务被认领）是两件不同的事，同时成立是正常的，不是冲突。查看器里的 `RUN`/`BLOCK`/`DECIDE`/`MERGE`/`DONE` 徽标取的是 SteerLine 的**执行/交付**轴，**不是** Beads 状态。

## 权威归属

- **执行态与交付态**：SteerLine 唯一权威（`run_dir/steerline.db`）；`runtime.json` v5 是可重建的观察投影。v1–v4 runtime 不兼容，服务拒绝恢复且保留原文件。
- **任务认领与关闭**：Beads 项目仍由 Beads 权威；native 项目的 Task、claim 和 delivery 记录由中心 SQLite 权威。native 项目默认 manual/paused；`task validate/create/discard` 使用中心命令事务创建入图和显式丢弃，`task show/list/graph/export` 从同一数据库读取依赖与阻塞。关系视图最多返回 500 个节点并明确标记边界；JSON 导出是只读快照，不能作为批量创建输入。领取只建立固定 WorkflowRun/Execution/Attempt 和准备意图，仍需生产原生执行接入后才能启动 worker；delivery 只有在固定门槛通过且平台确认真实 merged PR 后提交。
- **PR 合并**：外部平台权威；SteerLine 查询核实后才推进交付态。
- **派发与服务命令**：SQLite 中的服务状态、CommandService 回执与控制检查点权威。manual 不自动 claim；pause 只挡新派发；停止服务与关闭查看器分开。控制请求 `applied` 只说明指令完成；`answer_decision` 的 `accepted` 只说明答案已保存。两者都不代替 run 的执行或交付状态。

## 生命周期（一次运行）

```
新建:        execution=pending    finish=pending        beads=open
claim ok:                                               beads=in_progress
worker 起:   execution=running    finish=pending
正常退出 + pr_submitted:  execution=succeeded  finish=waiting_pr_merge
正常退出 + needs_decision: execution=succeeded finish=waiting_decision  decision.pending
  命令接受:               decision.answer_saved (答案已固定，尚未续跑)
  续跑准备:               decision.resuming → reply_accepted (后端已接受)
  匹配原问题与固定答案证明: decision.applied
  执行结束但缺证明:        execution=failed    finish=blocked  decision.failed
  （再到 pr_submitted / blocked）
正常退出 + blocked/无效结果: execution=succeeded/failed  finish=blocked
PR 已合并 + 结项命令 + bd close:  finish=completed   beads=closed
重启: pending/running → unknown/blocked（不自动续跑）
      waiting_decision → 保持等待；若停在 resuming 或状态缺失 → blocked
```

## Beads 的定位（过渡性任务提供方）

长期方向是**移除 Beads、把 DAG 收进 SteerLine**。因此：

- `beads_status` 只是**外部任务提供方的镜像**，将来会由 SteerLine 自己的任务/DAG 态（如 `task_status`）取代；上面的三个轴不变。
- `Beads` 在代码里应被视为"任务提供方的一种实现"，而不是状态模型的根基。移除时替换的是**任务/认领轴的来源**，执行态与交付态不动。

## 已知限制（过渡期）

`beads_status` 是**惰性镜像**：在 claim、结项、以及 `reconcile` 时刷新；而 `reconcile` 只遍历 `Beads.Owned`（当前 claimant 名下的任务）。因此若 claim 被他人抢走或任务在 Beads 侧被改动，任务不再出现在 `Owned`，`beads_status` **可能不刷新**（漂移）。

- 这是**过渡期**问题：一旦任务状态归 SteerLine 自己持有，漂移不再存在。
- 现阶段**有意不对账**：为即将移除的 Beads 增加对账/漂移检测投入与目标相悖。运维上以 Beads 侧为准；需要时人工核对。
- 若在移除 Beads 之前必须更安全，最小代价是在 `reconcile` 里对本地所有未完成 run 调 `bd show` 比对——列为可选后续项，不做默认投入。

## 源码 / 二进制 / 状态 三分与 STEERLINE_HOME

三种东西本质不同，不应共用一套路径习惯：

| 类别 | 位置 | 生命周期 |
| --- | --- | --- |
| 源码 | 任意 git worktree / 分支 | 临时，可随时丢弃 |
| 二进制 | `%STEERLINE_HOME%\bin\steerline.exe` | 稳定；`steerline install` 升级 |
| 配置 | `%STEERLINE_HOME%\etc\<repo>.json` | 每个目标仓库一份 |
| 状态 / 数据 | `%STEERLINE_HOME%\state\<repoKey>\runtime.json` + `runs\` | 本机该仓库的唯一真相 |

`STEERLINE_HOME` 默认 `C:\SteerlineRuntime`（同名环境变量可覆盖）。

`repoKey` 由仓库的 Git common-dir 派生：`<name>-<shortHash(commonDir)>`。**同一仓库的所有 worktree/clone 的 common-dir 相同**，因此：

- 从任何 worktree 运行，`run_dir` 都解析到同一个 `state\<repoKey>`；
- 配置可省略 `run_dir`，自动派生（`worktree_root` 缺省为 `<run_dir>\worktrees`）；显式写 `run_dir` 仍然生效，向后兼容。

`task_authority=native` 配置必须省略 `run_dir` 并使用仓库派生的唯一状态目录，防止同一仓库通过不同配置分叉任务权威。该目录首次初始化时必须为空，仓库不得含 Beads 状态；已有数据库中的 authority 与 repository/remote/branch 必须匹配配置。Beads 配置不受这一 native 初始化规则影响。

命令：

- `steerline home -config ...`：打印 home / bin / etc / state / repository / run_dir。
- `steerline install`：把当前二进制拷到 `%STEERLINE_HOME%\bin\steerline.exe`（目标被运行中的服务占用时会报错，先用 `steerline stop-service` 停服并核验退出）。
- `steerline start-service -config ...`：Windows 离线时返回固定 `service-window.bat` 与配置绝对路径，由用户独立打开可见服务窗口；已有兼容实例则连接并保留现有模式。Linux 保留独立启动能力。
- `steerline stop-service -config ...`：提交 `stop_service`，并等待终态回执、原服务进程退出及仓库/run_dir 锁释放。`steerline stop` 仍是旧 `stop.json` 兼容入口。

一句话：**改代码在任意 worktree，`steerline install` 一次；服务和查看器永远跑 `bin\steerline.exe`，状态永远在 `state\<repoKey>\`。**

## 服务窗口身份与启动记录

每次 `run` 启动的尝试记录保存在 `run_dir/service/startups/<startup_id>/startup.json`，日志为同目录的 `service.log`。Windows 服务窗口进程自行生成 startup_id，窗口状态摘要也写入该日志。记录绑定 repository identity/path、run_dir、remote、branch、claimant、配置摘要、startup_id、实例 ID 和 OS 进程身份；它只识别启动尝试，不代替 runtime。startup_id 只接受 32 位小写十六进制，输出路径必须留在本次启动目录中。

Windows 服务窗口没有另外的服务启动父进程；窗口中的服务在 preflight 前发布自己的身份，之后独占 `startup.json` 生命周期记录。Linux 独立启动路径的父进程仍将 OS 身份写入 v1 `process.json`，不能覆盖子进程已发布的 `started` 或 `failed`。客户端确认进程已退出时可将未确认的启动标记为失败；损坏或多个仍存活的启动记录会阻止新的启动。

服务连接只接受 runtime v5。控制协议仍独立使用 v1；不兼容 runtime 版本不能提交控制请求，也不会因进程探测或旧停服记录而升级为受支持状态。

连接读取 runtime 快照、新鲜度、进程探测、配置摘要和控制协议能力，并单独呈现。进程探测区分匹配、确定不存在、PID 已复用和无法核验；权限不足及身份不明不等于离线。过期 heartbeat、ready=false 和协议不兼容均保留旧实例，不自动另起服务。配置摘要变化时保留观察与显式停服能力，并禁用其他控制动作。控制台退出、连接断开和等待超时都不会停止服务。

Windows 的服务窗口直接竞争 Git common-dir 仓库锁和 run_dir 锁；两配置指向同仓库但使用不同 run_dir 时，仓库锁拒绝第二个窗口。客户端只在同一 startup_id、进程身份匹配且 ready=true 的 runtime 快照出现后显示已连接；初始化超时保留待确认状态，不自动打开替代窗口。`service/launch.lock` 仅供仍使用独立启动的 Linux 客户端串行化同一 run_dir。

Windows 的 `service-window.bat` 从稳定安装 exe 以 manual/summary 模式运行。服务必须有可见 console 窗口，运行时监测其窗口句柄；窗口消失时取消服务上下文并清理 worker。该模式不要求从祖先 Job Object 脱离，关闭独立 viewer/TUI 也不会关闭服务窗口。Linux 服务以新会话启动，不使用 worker 的父死亡信号；同版本服务连接时保留原 mode/pause。

Windows worker 子进程仍先以挂起状态创建，纳入 kill-on-close Job 并核验身份后才恢复执行；该 Job 在服务窗口异常退出时清理 worker。停服请求固定指向连接时观察到的实例，结果始终包含 request_id 和 instance_id。即使没有回执，也独立报告原进程是否退出；只有 applied、进程退出和两把锁释放均确认后才报告 stopped。历史快照中的进程已退出但服务锁尚未释放时，不开放重新启动。

## 多项目

- 每个目标仓库一份 `etc\<repo>.json`；`run_dir` 省略时各自派生到 `state\<repoKey>`，互不干扰。
- 并行：一个项目一个服务进程；锁按仓库（git common-dir）和 run_dir，不同仓库可同时跑。同一项目的 `start-service` 连接已有兼容服务，Windows 离线时提示打开窗口；重复打开的服务窗口由仓库锁拒绝。
- 入口：`steerline projects`（交互选项目/动作）、`steerline launchers`（生成双击 `.bat`）。查看器目前一个窗口看一个项目（标题带 repo）。
- 目标仓库需先在 Orca 注册。

## 相关文档

- 数据契约字段细节：[data-contract](data-contract.md)
- 人工决策：`waiting_decision` 与 `decision.*`：[decisions](decisions.md)
- 活动事件：`docs/activity.md`
