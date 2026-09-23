# Activity 契约：activity.jsonl 与 steerline watch

本文是**稳定读取契约**。`activity.jsonl` 是 agent 原始流（`events.jsonl`）的归一化派生物，面向人和外部工具，schema 独立于 codex / pi。渲染样式不是契约，可随时变化；JSONL 字段和语义是契约。

## 目的与位置

- 每个运行一个文件：`<run_dir>/runs/<run_id>/activity.jsonl`；retry 写在 `attempts/<attempt_id>/activity.jsonl`（与当次 `events.jsonl` 同目录）。
- 派生自 `events.jsonl`（原始取证，永不改动）和调度器生命周期。删掉 `activity.jsonl` 不影响任何恢复：它可随时由 `events.jsonl` 重放重建。
- **单写者**：只有服务写该文件。任何宿主（本地进程、Orca）都不直接写它。
- **按 run 隔离**：每个 run 一个 writer，PR/合并/结项等 worker 退出后的事件仍写入**该 run 自己**的文件，不会串到别的 run。
- **追加、不截断**：worker 退出后 writer 不关闭，重启后按需重开并续写；`seq` 从文件里已存在的最大值继续，末尾半行会在重开时修复。
- **每行 flush**，读者可近实时跟随。

## 记录 schema（v1）

每行一个 JSON 对象：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| v | integer | schema 版本，当前 1 |
| seq | integer | 本文件内单调递增，从 1 开始，用于排序与去重 |
| ts | timestamp | 记录时间（RFC3339 UTC）。agent 事件无自带时间，取到达时间 |
| source | string | `codex` / `pi` / `steer`（调度器生命周期） |
| run_id / task_id | string | 运行的稳定身份，由写者补齐 |
| item_id | string/null | agent 内同一个工具调用/消息的关联键，用于把 running 更新折叠到同一项 |
| kind | string | 见下表 |
| status | string/null | `started` / `running` / `completed` / `failed` / `info` |
| title | string/null | 简短标题（命令首行、工具名、文件变更列表、生命周期标题） |
| detail | string/null | 正文或输出，最多 4096 字符（原始全文在 events.jsonl） |
| elapsed_ms | integer/null | 成对的 started→completed 时长 |
| meta | object/null | 结构化附加信息，键不稳定，读者按需取 |
| raw | object/null | 无法归一化时保留的原始行（前向兼容） |

### kind 枚举

| kind | 含义 |
| --- | --- |
| session | agent 会话开始（含 thread/session id） |
| turn | 一轮开始/结束 |
| message | 助手可见消息 |
| reasoning | 思考摘要（可能为空，取决于 runner 配置） |
| command | 工具/命令执行 |
| file_change | 文件改动 |
| error | runner 或工具的失败 |
| usage | token 用量 |
| note | compaction、队列等提示 |
| unknown | 未识别的类型，保留 `raw` |

调度器生命周期额外使用 `claim`、`prepare`、`worker_start`、`worker_exit`、`result`、`retry`、`pr`、`completion`。

### 关键规则

- **前向兼容**：新的 agent 事件类型映射为 `kind=unknown` 并原样放进 `raw`，绝不报错、绝不丢行。读者必须容忍未知字段与未知 kind。
- **更新语义**：runner 可能对同一 `item_id` 先后发出 `running` 与 `completed`。TTY 渲染可原地替换该行；JSONL 消费者按 `item_id` 自行合并。codex 通常只有 started/completed，pi 有增量 update。
- **有界**：`detail` 截断到 4096 字符，`title` 截断到 240 字符；完整内容在 `events.jsonl`。
- 读取规则与 runtime 相同：容忍末尾半行、文件被截断或原子替换；收到 `seq` 回退时应重开文件。

## 稳定入口：steerline watch

```
steerline watch -config <config.json> [--run-id <id|task-id>] [--follow=true] [--from-start=true] [--json] [--raw]
```

- 不依赖服务在跑，也不改任何状态：只读 `runtime.json` 和运行文件。
- 未给 `--run-id` 时选当前 `execution_status=running` 的运行，否则选最后一个。
- 默认读 `activity.jsonl`；若不存在则回退到把 `events.jsonl` 现场归一化（兼容旧二进制与原始宿主采集）。
- `--json` 输出稳定 schema（`activity.jsonl` 行原样透传；回退路径则输出归一化记录）。
- `--raw` 输出 `events.jsonl` 原始行。
- 退出：读到当前末尾即返回；`--follow` 时持续跟随直到 Ctrl+C。服务停止不影响 watch 读已有文件。

## 服务端控制台（同一渲染器）

`steerline run -console pretty|summary|json|off`。TTY 下默认 `pretty`；`summary` 只打生命周期；`json` 打稳定 schema；`off` 静默。控制台输出是**视图**，不是契约。

## 示例

codex 原始行 → activity：

```
{"type":"thread.started","thread_id":"01a0..."}
→ {"v":1,"seq":1,"ts":"...","source":"codex","run_id":"3f9a","task_id":"bd-142","kind":"session","status":"info","title":"codex session","meta":{"thread_id":"01a0..."}}

{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"ok\n","exit_code":0,"status":"completed"}}
→ {"v":1,"seq":4,"ts":"...","source":"codex","run_id":"3f9a","task_id":"bd-142","item_id":"item_1","kind":"command","status":"completed","title":"go test ./...","detail":"ok\n","elapsed_ms":13200,"meta":{"exit_code":0}}
```

pi 增量：

```
{"type":"tool_execution_start","toolCallId":"tc1","toolName":"bash","args":{"command":"ls"}}
→ {... "source":"pi","item_id":"tc1","kind":"command","status":"started","title":"bash" ...}
{"type":"tool_execution_update","toolCallId":"tc1","toolName":"bash","partialResult":"a\nb\n"}
→ {... "source":"pi","item_id":"tc1","kind":"command","status":"running","title":"bash","detail":"a\nb\n" ...}
```

## 版本化

- 破坏性字段变更时提升 `v`，并保留旧版本读取路径。
- 允许新增可选字段与新增 kind/status；读者不得因未知值而失败。
