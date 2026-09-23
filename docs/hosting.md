# Hosting 红线 v2：执行交给 Orca，调度与完成留在 Steerline

v1 的红线是按"Orca 只当哑终端宿主"写的；目标改为**把执行切到 Orca** 后，第 2、4 条（以及第 3、5 条）必须修正。本文是当前有效的红线。

## 职责划分

**Orca 负责执行**：创建 worktree、启动 agent（codex/claude/pi/…）、跑 agent、提供实时终端/输出、agent 存活与终端状态。

**Steerline 负责调度与完成**：Beads 领取与状态、任务选择、DAG/编排、PR 合并核实、Beads close、恢复策略。

| 事实 | 归属 |
| --- | --- |
| 任务领取 / Beads 状态 | Steerline |
| worktree 路径 / 分支 / agent 存活 / 终端 | **Orca**（Steerline 只读校验 branch、HEAD、base 祖先） |
| PR 是否合入目标主线 | 外部平台（Steerline 核实） |
| 最终完成 / Beads close | Steerline |
| 编排 / DAG / 人机消息 | Steerline |

原则：**执行事实以 Orca 为准，任务与完成事实以 Steerline 为准。** v1 的"Orca 不是任何事实源"不再适用。

## 允许的 Orca 命令

- `status`
- `repo list|show`
- `worktree list|show|ps|create|rm|set`
- `terminal create|show|read|wait|send|switch|close`
- `agent hooks status`

## 禁止的 Orca 命令

- `orca orchestration *`（`run-create`、`task-create`、`worker-start`、`dispatch`、`send`、`check`、`ask/reply`、`gate-*`）。Run/Task/Dispatch 概念不进入 Steerline；编排/DAG 留在 Steerline。
- `automations`、`browser`、`computer`、`artifacts`、`publishing`、emulator。

## 结果契约（R-a）

Orca 启动的是 TUI agent，没有 `--output-schema` / 进程退出码。因此：

- prompt 用 Orca 原生 `--prompt` prefill 下发（pointer→`prompt.txt`），不用启动后再按键。
- agent 按 prompt/skill 在 worktree 内写结果文件（`<worktree>/.steerline-result.json`，prompt 里给的是相对 CWD 路径）。
- Steerline 读取、用内嵌 `result.schema.json` 校验，再落到权威 `runs/<run_id>/result.json`。
- `outcome=needs_decision`：agent 无法继续、需要人工决策时，带 `decision{question,options[],recommended,allow_free_text}`；Steerline 置 `finish_task_status=waiting_decision`，保留 claim/worktree/终端并暂停新派发，等人回答后续跑。详见 [decisions](decisions.md)。
- **结果文件不是完成**：仍需 Steerline 核实 PR 已合入目标主线并确认 Beads closed。

## fallback 边界

- 只有在**尚未让 Orca 创建 worktree** 之前，Orca 不可用才可按 `fallback` 回退本地或失败。
- 一旦 Orca 建了 worktree，Orca 不可达就是 `unknown/blocked`，**不本地重造、不自动续跑**。

## 交互纪律

- TUI agent 可能停在需要人工的提示；用 `terminal show` 的 `agentWait` 检测，判为 `blocked` 并保留终端，**不代答**。
- 需要人工处理走 Steerline 恢复命令（`stop`/`mark-stopped`/`accept-result`/`retry`），不在 agent 终端里回答。
- 手动 `terminal send` 属于 out-of-band，该运行确定性视为失效。
- pi 前置：pi 首次进入未信任目录会弹 `Trust project folder?`，截获 prefill 提交键。预先在 `~/.pi/agent/trust.json` 信任 `<orca>/workspaces/<repo>`（pi 向上查找父目录），否则该次 dispatch 收不到 prompt。

## 活动流说明

Orca TUI 没有结构化事件流，`activity.jsonl` 退化为：Steerline 生命周期事件 + 从 `terminal read` 尽力归一化的文本；它不再是完整逐项事件。这是把执行交给 Orca 的代价。

## 评审检查清单

- [ ] 是否调用了 `orca orchestration *`？（禁止）
- [ ] 是否把 worktree/agent 存活当成 Steerline 的判定？（应读 Orca）
- [ ] 是否把结果文件当成了"完成"？（应再验 PR + Beads）
- [ ] Orca 建了 worktree 后是否错误地回退本地？
- [ ] `activity.jsonl` 是否仍只有服务一个写者？
