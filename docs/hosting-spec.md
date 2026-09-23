# Hosting 规范 v2：Orca 驱动执行

边界见 [hosting](hosting.md)。本文是实现规范。v2 起执行模型为 **Orca 建 worktree + Orca 启 agent**；Steerline 只做调度与完成。

## 配置轴

```
host  = local | orca     在哪执行（默认 orca）
agent = codex | pi | ... 交给 Orca 的 --agent id（默认 codex，仅 orca host）
```

`runner` 轴已删除：选择哪个 agent 由 Orca 的 `--agent` 决定，Steerline 不再拼 agent 命令、不再有 pi runner。

## Orca 执行流程（host=orca）

```
1. Steerline 领取 Beads 任务，fetch 主线，得到 base
2. 持久化运行与意图（branch/base；worktree 路径由 Orca 决定，创建后回填）
3. orca worktree create --name <task> --agent <agent> --base-branch <baseRef> \
     --prompt "读取并执行 <run_dir>/prompt.txt" --json   # Orca prefill 提交，不用启动后按键
   → worktreePath / worktree branch / agent terminal handle
4. 回填并持久化 worktree_path / worktree_branch / base_commit / process(orca)
5. 写 <run_dir>/prompt.txt（含 worktree 事实）与 result.schema.json
   —— 只把 pointer 交给 --prompt；prompt 正文在 create 返回后写，远早于 agent TUI 就绪
6. 监管：轮询结果文件 + terminal show(agentWait)
7. 结果文件出现 → 校验 schema → 落 runs/<run_id>/result.json
8. Steerline 核验 PR 合入 + Beads closed（既有逻辑）
```

### 为什么不 `terminal send`

- `terminal wait --for tui-idle` 会在 agent **启动菜单**阶段就返回，随后的 `terminal send` 会被启动界面吞掉（实测 pi：pointer 从未送达，agent 空转）。
- 改用 Orca 原生 `--prompt`：Orca 在 agent 就绪后 prefill 并提交，无竞态。pointer 只引用 `prompt.txt`，正文如常在 create 之后写。

### pi 前置：信任目录

- pi 首次进入未信任目录会弹 `Trust project folder?`，会截获 Orca prefill 的提交键，导致这次运行收不到 prompt。
- pi 的信任解析会**向上查找父目录**（`findNearestTrustEntry`）。预先在 `~/.pi/agent/trust.json` 加 `"<orca>/workspaces/<repo>": true`，此后新建 worktree 不再弹框。
- 这是 pi 的环境前置，不是 Steerline 逻辑；codex 等无此门槛。

### 结果契约 R-a

- agent 写 `<worktree>/.steerline-result.json`（worktree 内，避免沙箱越界）。
- Steerline 读取、校验（沿用 `result.schema.json`），原子写入 `runs/<run_id>/result.json`，之后走既有 `ReadResult`/`verifyRunWorkspace`/`bind`。
- prompt 模板增加 Orca 专属段：agent 的 CWD 即 worktree 根，结果写到其中的相对路径 `.steerline-result.json`；"必须是纯 JSON"。

### 完成检测

- 主判据：结果文件出现且校验通过。
- 辅助：`terminal show` 的 `agentWait != null` → 判 `blocked`（agent 在等人工），保留终端不代答。
- `stall_timeout`（默认 5m）：既无结果文件、无终端新输出、无 agentWait 变化 → `blocked`。
- 不把 `tui-idle` 当完成：agent 可能多轮。

### 回收

- `Kill()`：`terminal close`；可选 `worktree rm`（保留现场优先，默认不 rm）。
- Steerline 停止/超时：关闭终端并保留 worktree 供诊断。

## 恢复

- 重启见到 `host=orca` 的 pending/running → `unknown/blocked`，绝不重连续跑。
- 诊断：`worktree show --worktree path:<path>` + `terminal show --terminal <handle>`；`incarnationId` 变化或不可达 = unverifiable。
- 结果文件与 worktree 在本地磁盘，`exit.json`/`result.json` 概念对 Orca 变为 `.steerline-result.json`；本地 host 仍用 `exit.json`。
- 本地 host 的 `accept-result` 逻辑保持：人工核对后显式接回。

## 配置增量

```json
"host": "orca",
"agent": "codex",
"stall_timeout": "5m",
"orca": {
  "executable": "orca",
  "args": [],
  "shell": "cmd.exe",
  "wait_timeout": "15m",
  "keep_terminal": false,
  "fallback": "local"
}
```

- `runner`、`model`、`profile`、`sandbox`、`codex` 命令对 orca host 不再使用（agent 的模型/权限由 Orca 自身配置）。
- 默认值 `host=orca`、`agent=codex`。

## runtime v3

- `schema_version=3`；run 增 `host`、`process.kind/terminal/runtime_id/incarnation`。
- orca 的 `worktree_path`/`worktree_branch` 来自 Orca；`base_commit` 仍是 Steerline 固定并核验的祖先。
- v1/v2 记录视为 `host=local`。

## 分期

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| P0 | `activity.jsonl` + normalizer + `steerline watch` + `run -console` | 已实现 |
| P1 | `Spawner`/`Worker` 抽象 + `LocalSpawner` | 已实现（保留给 local host） |
| P2-v1 | Orca 当可见终端宿主（headless worker） | 已被 v2 取代 |
| P2-v2 | Orca 建 worktree + `--agent` 启 agent + R-a 结果 | 已实现（含集成测试） |
| P3 | 远程 host（`--on`/`--environment`）+ 结果回传 | 待做 |

> 迁移说明：`host=local` 路径与旧测试保留，作为无 Orca 环境与回归基线；`host=orca` 走 v2 模型。
>
> 已真机验证（Orca 1.4.206）：`--base-branch refs/steerline/bases/<runID>` 完整 ref 被接受，返回的 `worktree.head` 等于该 ref 指向的 pinned base，`baseRef` 原样保留。agent handle 读取容错：先 `agentTerminalHandle`，再 `startupTerminal.handle`，最后 `terminal list --worktree path:`。
> 集成测试用的是协议替身（`git worktree add` + 写 `.steerline-result.json`），未连真实 Orca runtime；`--agent` 的真实返回形状依赖上述容错。
