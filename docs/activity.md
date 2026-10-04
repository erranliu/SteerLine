# Activity 契约：规范事件与诊断镜像

产品活动和会话只读取 SQLite 中已提交的规范记录。activity.jsonl 是当前活动的可选诊断镜像，不是产品读取或恢复来源。原生历史/日志由对应会话采集器解释；已退役的 Pi/旧 exec stdout normalizer、格式探测和 raw fallback 不再提供。

## 目的与位置

- 每个运行一个文件：`<run_dir>/runs/<run_id>/activity.jsonl`；retry 写在 `attempts/<attempt_id>/activity.jsonl`（与当次 `events.jsonl` 同目录）。
- 派生自 `events.jsonl`（原始取证，永不改动）和调度器生命周期。删掉 `activity.jsonl` 不影响业务恢复；当前产品不从原始文件自动重建该镜像。
- **单写者**：只有服务写该文件。任何宿主（本地进程、Orca）都不直接写它。
- **按 run 隔离**：每个 run 一个 writer，PR/合并/结项等 worker 退出后的事件仍写入**该 run 自己**的文件，不会串到别的 run。
- **不承载控制审计**：control 请求和回执保存在 run_dir 的 `control/requests` 与 `control/receipts`；删除或重建 activity 不会重放、确认或改变控制指令结果。
- **追加、不截断**：worker 退出后 writer 不关闭，重启后按需重开并续写；`seq` 从文件里已存在的最大值继续，末尾半行会在重开时修复。
- **每行 flush**，读者可近实时跟随。

PR 4B 增加了单独的 `ConversationEvent` 读模型，保存在当前 `steerline.db` 中，完整消息正文与 raw 行按摘要/修订引用读取。产品活动视图使用规范数据库事件，`activity.jsonl` 仅供手动诊断，其 4096 字符 detail 上限不作为会话正文上限。两者不互相导入或重建。

## 记录 schema（v1）

每行一个 JSON 对象：

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| v | integer | schema 版本，当前 1 |
| seq | integer | 本文件内单调递增，从 1 开始，用于排序与去重 |
| ts | timestamp | 记录时间（RFC3339 UTC）。agent 事件无自带时间，取到达时间 |
| source | string | 实现声明的来源或 `steer`（中心生命周期） |
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
- **更新语义**：runner 可能对同一 `item_id` 先后发出 `running` 与 `completed`。TTY 渲染可原地替换该行；JSONL 消费者按 `item_id` 自行合并。具体状态由当前原生采集器提供，不通过日志格式探测选择解析器。
- **有界**：`detail` 截断到 4096 字符，`title` 截断到 240 字符；完整内容在 `events.jsonl`。
- 产品通过 Activity 查询规范数据库事件，以明确的 Run/Execution/Attempt 和 EventID 游标读取。采集缺口明确显示，不使用原始日志或诊断镜像补画。
- 会话文件采集保留独立的通用 JSONL framing：只消费换行结束的完整记录，offset 按实际字节计算，处理 UTF-8/CRLF、文件截断与原子替换。每批条数/字节、单记录与扫描预算保持有界；超长行诊断后继续，原始文件不改写。通用读取器不解释活动 seq 或检测 agent 格式。
- 旧 steerline watch 及自动 raw fallback 已退役，当前 CLI 不提供该入口。镜像格式和服务端控制台输出保持现行用途。

## 服务端控制台（同一渲染器）

`steerline run -console pretty|summary|json|off`。TTY 下默认 `pretty`；`summary` 只打生命周期；`json` 打稳定 schema；`off` 静默。控制台输出是**视图**，不是契约。

`json` 模式的 stdout 只含 JSONL，不混入人类可读的启动提示。每行包含活动版本、时间和当前服务输出流内递增的 `seq`；此序号独立于各 run 的 `activity.jsonl`，不会改写日志中的序号。

## 示例

中心写入的诊断镜像行示例（产品读取时以数据库 EventID 排序）：

```json
{"v":1,"seq":1,"ts":"2026-10-04T00:00:00Z","source":"steer","run_id":"original-run","task_id":"original-task","kind":"decision","status":"pending","title":"worker needs input","meta":{"decision_id":"original-decision"}}
```

实际 agent 事件和原始取证由当前实现的会话 source/mapper 提供，本文不定义从旧 stdout 格式重建活动的转换规则。

## 版本化

- 破坏性字段变更时提升 `v`；依照项目规则明确拒绝旧开发格式，不新增兼容读取或重放路径。
- 允许新增可选字段与新增 kind/status；读者不得因未知值而失败。
