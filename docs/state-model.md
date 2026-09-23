# 状态模型：SteerLine 三个轴与 Beads 的关系

同一任务在 SteerLine 和 Beads 里各有一份"状态"，容易被误读成"两套矛盾的状态"。它们其实是**不同轴**：一个是进程/交付生命周期，一个是任务认领工作流。本文固化这套模型，并说明 Beads 的定位（过渡性任务提供方）。

## 三个正交轴

| 轴 | 字段 | 取值 | 权威 | 回答的问题 |
| --- | --- | --- | --- | --- |
| 执行 | `execution_status` | pending / running / succeeded / failed / unknown | SteerLine | agent 进程/终端在跑吗？正常退出吗？ |
| 交付 | `finish_task_status` | pending / waiting_decision / waiting_pr_merge / completed / blocked | SteerLine | 这次运行在等决策、等合并，还是已完成/阻塞？ |
| 任务/认领 | 当前 `beads_status`（镜像），将来 SteerLine 任务态 | open / in_progress / closed | 外部任务提供方（当前 Beads） | 任务被认领了吗？谁认领？关闭了吗？ |

另有独立的 `pr.state`（open / merged / closed_unmerged / unknown，平台权威）与 `decision.status`（pending / resuming / applied，见 [decisions](decisions.md)）。

服务还有独立的控制轴 `control.dispatch_mode`（manual/auto）、`dispatch_paused` 和 `revision`。它们控制将来是否领取新任务，不改变执行、交付或 Beads 认领事实。`control.operation` 和 `control.blockers` 是控制协议的持久检查点/不确定副作用保留，不是任务队列；服务是 runtime 的唯一写者，CLI 仅提交文件请求并读取回执。

**关键**：`execution_status=running`（进程在跑）与 Beads `IN_PROGRESS`（任务被认领）是两件不同的事，同时成立是正常的，不是冲突。查看器里的 `RUN`/`BLOCK`/`DECIDE`/`MERGE`/`DONE` 徽标取的是 SteerLine 的**执行/交付**轴，**不是** Beads 状态。

## 权威归属

- **执行态与交付态**：SteerLine 唯一权威（`runtime.json`）。
- **任务认领与关闭**：当前由 Beads 权威；SteerLine 通过 `bd claim` / `bd close` **写入**，并在完成后以 `bd show` 回读核实（不以本地预测替代 `closed`）。
- **PR 合并**：外部平台权威；SteerLine 查询核实后才推进交付态。
- **派发控制**：当前 runtime v4 的服务主状态机权威。manual 不自动 claim；pause 只挡新派发；停止服务与关闭查看器分开。控制请求的 `applied` 只说明指令完成，不代替 run 的执行或交付状态。

## 生命周期（一次运行）

```
新建:        execution=pending    finish=pending        beads=open
claim ok:                                               beads=in_progress
worker 起:   execution=running    finish=pending
正常退出 + pr_submitted:  execution=succeeded  finish=waiting_pr_merge
正常退出 + needs_decision: execution=succeeded finish=waiting_decision  decision.pending
  人工回答 → 续跑:        execution=running    finish=waiting_decision  decision.applied
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

命令：

- `steerline home -config ...`：打印 home / bin / etc / state / repository / run_dir。
- `steerline install`：把当前二进制拷到 `%STEERLINE_HOME%\bin\steerline.exe`（目标被运行中的服务占用时会报错，先用 `steerline stop-service` 停服并核验退出）。
- `steerline start-service -config ...`：从稳定安装路径显式启动独立服务，新实例为 manual 派发模式；已存在兼容实例则连接并保留现有模式。
- `steerline stop-service -config ...`：提交 `stop_service`，并等待终态回执、原服务进程退出及仓库/run_dir 锁释放。`steerline stop` 仍是旧 `stop.json` 兼容入口。

一句话：**改代码在任意 worktree，`steerline install` 一次；服务和查看器永远跑 `bin\steerline.exe`，状态永远在 `state\<repoKey>\`。**

## 后台服务身份与启动记录

`start-service` 的启动尝试记录保存在 `run_dir/service/startups/<startup_id>/startup.json`，独立日志为同目录的 `service.log`。记录绑定 repository identity/path、run_dir、remote、branch、claimant、配置摘要、startup_id、实例 ID 和 OS 进程身份；它只用于识别启动尝试，不代替 runtime。startup_id 只接受 32 位小写十六进制，输出文件路径必须留在本次启动目录中。

父进程只在创建子进程前初始化 `startup.json`，创建后将 OS 身份写入独立的 v1 `process.json`；子进程在 preflight 前发布自己的身份，之后独占生命周期记录的更新。父进程不会用旧的 `starting` 记录覆盖子进程已发布的 `started` 或 `failed`。客户端确认进程已退出时可将未确认的启动标记为失败；损坏或多个仍存活的启动记录会阻止新的启动。

对于 PR 2 的 v4 服务（没有 `startup_id`），配置摘要变化时，连接端还要求匹配的 OS 身份及 repository/run_dir 两把锁，才能保留显式 stop 能力。停服客户端确认 `stop_service=applied`、原进程退出和两把锁释放后，会在 `service/stops/<instance_id>.json` 写入版本化停服观察记录，证明该旧实例原本属于此 run_dir；这使已停服务在修改配置摘要后仍可显式重新启动，不改 runtime v4。

连接读取 runtime 快照、新鲜度、进程探测、配置摘要和控制协议能力，并单独呈现。进程探测区分匹配、确定不存在、PID 已复用和无法核验；权限不足及身份不明不等于离线。过期 heartbeat、ready=false 和协议不兼容均保留旧实例，不自动另起服务。配置摘要变化时保留观察与显式停服能力，并禁用其他控制动作。控制台退出、连接断开和等待超时都不会停止服务。

`service/launch.lock` 只串行化同一 run_dir 下的启动客户端；服务仍分别持有 Git common-dir 仓库锁和 run_dir 锁。两配置指向同仓库但使用不同 run_dir 时，仓库锁拒绝第二个服务。启动请求返回成功只代表子进程已创建；客户端须收到同一 startup_id、身份匹配且 ready=true 的 runtime 握手。30 秒超时会保留该 ID；后续启动调用观察原尝试，不会因为超时重复启动或杀掉它。

Windows 后台启动要求服务进程离开启动它的 Job Object，并脱离父控制台；创建后还会核验 Job Object 隔离。若宿主不允许 breakaway，明确返回错误。Linux 服务以新会话启动，不使用 worker 的父死亡信号。新启动使用 manual 派发；兼容服务连接时保留它的 mode/pause。

Windows 子进程先以挂起状态创建，完成 Job 隔离和 OS 身份核验后才恢复执行；隔离失败不会先执行服务恢复或 worker 逻辑。停服请求固定指向连接时观察到的实例，结果始终包含 request_id 和 instance_id。即使没有回执，也独立报告原进程是否退出；只有 applied、进程退出和两把锁释放均确认后才报告 stopped。历史快照中的进程已退出但服务锁尚未释放时，不开放重新启动。

## 多项目

- 每个目标仓库一份 `etc\<repo>.json`；`run_dir` 省略时各自派生到 `state\<repoKey>`，互不干扰。
- 并行：一个项目一个服务进程；锁按仓库（git common-dir）和 run_dir，不同仓库可同时跑。同一项目的 `start-service` 连接已有兼容服务，前台 `run` 的重复服务仍由仓库锁拒绝。
- 入口：`steerline projects`（交互选项目/动作）、`steerline launchers`（生成双击 `.bat`）。查看器目前一个窗口看一个项目（标题带 repo）。
- 目标仓库需先在 Orca 注册。

## 相关文档

- 数据契约字段细节：[data-contract](data-contract.md)
- 人工决策：`waiting_decision` 与 `decision.*`：[decisions](decisions.md)
- 活动事件：`docs/activity.md`
