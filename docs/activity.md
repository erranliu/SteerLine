# Activity 契约：activity.jsonl 与 steerline watch

本文是**稳定读取契约**。`activity.jsonl` 是 agent 原始流（`events.jsonl`）的归一化派生物，面向人和外部工具，schema 独立于 codex / pi。渲染样式不是契约，可随时变化；JSONL 字段和语义是契约。

## 目的与位置

- 每个运行一个文件：`<run_dir>/runs/<run_id>/activity.jsonl`；retry 写在 `attempts/<attempt_id>/activity.jsonl`（与当次 `events.jsonl` 同目录）。
- 派生自 `events.jsonl`（原始取证，永不改动）和调度器生命周期。删掉 `activity.jsonl` 不影响任何恢复：它可随时由 `events.jsonl` 重放重建。
- **单写者**：只有服务写该文件。任何宿主（本地进程、Orca）都不直接写它。
- **按 run 隔离**：每个 run 一个 writer，PR/合并/结项等 worker 退出后的事件仍写入**该 run 自己**的文件，不会串到别的 run。
- **不承载控制审计**：control 请求和回执保存在 run_dir 的 `control/requests` 与 `control/receipts`；删除或重建 activity 不会重放、确认或改变控制指令结果。
- **追加、不截断**：worker 退出后 writer 不关闭，重启后按需重开并续写；`seq` 从文件里已存在的最大值继续，末尾半行会在重开时修复。
- **每行 flush**，读者可近实时跟随。

PR 4B 增加了单独的 `ConversationEvent` 读模型，保存在当前 `steerline.db` 中，完整消息正文与 raw 行按摘要/修订引用读取。`activity.jsonl` 继续服务于人类活动视图，其 4096 字符 detail 上限不作为会话正文上限。两者不互相导入或重建。

## 记录 schema（v1）

每行一个 JSON 对象：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| v | integer | schema 版本，当前 1 |
| seq | integer | 本文件内单调递增，从 1 开始，用于排序与去重 |
| ts | timestamp | 记录时间（RFC3339 UTC）。agent 事件无自带时间，取到达时间 |
| source | string | `codex` / `pi` / `orca-supervised` / `steer`（调度器生命周期） |
| run_id / task_id | string | 运行的稳定身份，由写者补齐 |
| item_id | string/null | agent 内同一个工具调用/消息的关联键，用于把 running 更新折叠到同一项 |
| kind | string | 见下表 |
| status | string/null | `started` / `running` / `completed` / `failed` / `info` / `manual_host_check` |
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

调度器生命周期额外使用 `claim`、`prepare`、`worker_start`、`worker_exit`、`result`、`retry`、`pr`、`completion`、`decision`。

`decision` 表示 agent 请求人工决策以及该决策的流转（`status` 为 `pending`/`answer_saved`/`reply_accepted`/`applied`/`failed`）；`meta` 带 `decision_id`、`options`、`option_id`、`answered_by`。协议见 [decisions](decisions.md)。

受监管 Orca 的原始 Delivery 与完整 payload 先以 Run/Execution/Attempt、`external_message_id` 写入 SQLite；activity 只是展示摘要。`worker-read` 保留 `transcript` 或 `terminal` 的 source 与不透明 cursor，写入成功后才推进游标；source 改变、剪裁和历史缺口写显式 gap，不能以无输出推断 worker 完成。业务完成仍要求对应结构化 status、受接受的 worker_done 与中心门槛。

Orca 报告明确的 `agentWait` 对象，或 transcript 出现远端 helper 启动取消时，受监管 activity 记 `error/manual_host_check`，提示人工核实宿主交互；它不创建业务 Decision、不自动批准权限，也不把 `agentWait=null` 解读为无需人工检查。

### 关键规则

- **前向兼容**：新的 agent 事件类型映射为 `kind=unknown` 并原样放进 `raw`，绝不报错、绝不丢行。读者必须容忍未知字段与未知 kind。
- **更新语义**：runner 可能对同一 `item_id` 先后发出 `running` 与 `completed`。TTY 渲染可原地替换该行；JSONL 消费者按 `item_id` 自行合并。codex 通常只有 started/completed，pi 有增量 update。
- **有界**：`detail` 截断到 4096 字符，`title` 截断到 240 字符；完整内容在 `events.jsonl`。
- 读取器只提交换行结束的完整记录，offset 按字节计算并正确处理 UTF-8 与 CRLF。末尾半行会留待补齐；截断、同路径原子替换以及 `seq` 回退会启动新的逻辑读取代次。`seq` 回退从当前记录继续向前，物理 offset 不退回文件头。
- viewer 与 `watch` 共用有界 JSONL cursor 引擎。每批最多 256 条或约 1 MiB，单条上限 8 MiB；超长行会诊断并跳过到下一个换行，坏 activity JSON 诊断写 stderr。

## 稳定入口：steerline watch

中心生命周期与 worker 活动先随业务状态写入 SQLite events；QueryService 可在服务运行时按 Execution/Attempt 查询。`activity.jsonl` 是展示副本，写入失败不丢弃规范事件，重启不会把日志文件重新导入业务数据库。

```
steerline watch -config <config.json> [--run-id <id|task-id>] [--follow=true] [--from-start=true] [--json] [--raw]
```

- 不依赖服务在跑，也不改任何状态：只读 SQLite 中的运行检查点和运行文件。
- 未给 `--run-id` 时选当前 `execution_status=running` 的运行，否则选最后一个。每次调用固定所选 run 和解析出的日志路径，不会跟随 runtime 改选任务或 retry attempt。
- 默认读 `activity.jsonl`；若不存在则回退到把 `events.jsonl` 现场归一化（兼容旧二进制与原始宿主采集）。
- `--json` 输出稳定 schema（`activity.jsonl` 行原样透传；回退路径则输出归一化记录，并在本次调用中补齐 `v=1`、递增 `seq` 与 `ts`）。诊断始终写 stderr，不污染 JSON stdout。
- `--raw` 输出 `events.jsonl` 中上限内的完整原始行，不按 JSON 解析过滤。
- `--from-start=false` 从调用时文件末尾开始；若当时存在半行，会跳过到该行结束后再输出新记录。fallback 会读取首条完整原始记录来识别 Codex 或 Pi，即使实际消费从 EOF 开始。
- activity 初始缺失时本次调用固定读 raw fallback；后续 activity 文件出现时由新一次调用选择。viewer 会切换读取代次并替换旧 fallback 视图。
- 退出：读到当前末尾即返回；`--follow` 时持续跟随直到 Ctrl+C。服务停止不影响 watch 读已有文件。

## 服务端控制台（同一渲染器）

`steerline run -console pretty|summary|json|off`。TTY 下默认 `pretty`；`summary` 只打生命周期；`json` 打稳定 schema；`off` 静默。控制台输出是**视图**，不是契约。

`json` 模式的 stdout 只含 JSONL，不混入人类可读的启动提示。每行包含活动版本、时间和当前服务输出流内递增的 `seq`；此序号独立于各 run 的 `activity.jsonl`，不会改写日志中的序号。

## 示例

codex 原始行 → activity：

```
{"type":"thread.started","thread_id":"01a0..."}
→ {"v":1,"seq":1,"ts":"...","source":"codex","run_id":"3f9a","task_id":"142","kind":"session","status":"info","title":"codex session","meta":{"thread_id":"01a0..."}}

{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"ok\n","exit_code":0,"status":"completed"}}
→ {"v":1,"seq":4,"ts":"...","source":"codex","run_id":"3f9a","task_id":"142","item_id":"item_1","kind":"command","status":"completed","title":"go test ./...","detail":"ok\n","elapsed_ms":13200,"meta":{"exit_code":0}}
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
