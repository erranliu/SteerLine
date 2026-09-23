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
- `steerline install`：把当前二进制拷到 `%STEERLINE_HOME%\bin\steerline.exe`（目标被运行中的服务占用时会报错，先 `steerline stop`）。

一句话：**改代码在任意 worktree，`steerline install` 一次；服务和查看器永远跑 `bin\steerline.exe`，状态永远在 `state\<repoKey>\`。**

## 多项目

- 每个目标仓库一份 `etc\<repo>.json`；`run_dir` 省略时各自派生到 `state\<repoKey>`，互不干扰。
- 并行：一个项目一个 `steerline run` 进程；锁按仓库（git common-dir + run_dir），不同项目可同时跑，同一项目重复启动会被拒绝。
- 入口：`steerline projects`（交互选项目/动作）、`steerline launchers`（生成双击 `.bat`）。查看器目前一个窗口看一个项目（标题带 repo）。
- 目标仓库需先在 Orca 注册。

## 相关文档

- 数据契约字段细节：[data-contract](data-contract.md)
- 人工决策：`waiting_decision` 与 `decision.*`：[decisions](decisions.md)
- 活动事件：`docs/activity.md`
