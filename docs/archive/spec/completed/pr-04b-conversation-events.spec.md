# PR 4B Spec：统一会话事件与完整回复

任务 ID：1；任务名称：统一会话事件与完整回复。

状态：基础读模型与持久化已实现；E-A8 与 E-A9 已在报告限定的本机 Windows/Codex 双链路验收，其他故障场景仍有未完成项。依赖：PR 4A。对应 [Plan](pr-04b-conversation-events.plan.md)。下游：[4C 控制台](pr-04-console.spec.md)。验收状态见 [4B 验证报告](../../../validation/pr-04b-conversation-events-2026-09-24.md) 与[能力矩阵](../../../validation/pr-04b-conversation-capabilities-2026-09-24.json)。

## E1：语义与身份

所有后端向一个会话读模型提供事实。Session 是持续 agent 会话；Turn 是一轮交互；AssistantMessage 有 commentary / final_answer / unknown 阶段；ExecutionResult 是 SteerLine 业务核验的结构化结果。轮次结束、消息完成、步骤结束、业务任务完成分别记录，不能互相代替。

规范事件使用当前 envelope，至少保留 project、workflow/execution、attempt、backend/protocol、外部 ref、source segment、session/turn/item/source event 身份、本地序号、原始顺序/修订号（若有）、发生与接收时间及来源证据。未知 ID 允许为空并声明身份质量，不用标题/文本/时间猜测。身份与 4A 共用。

有可靠 turn 边界但无原生 turn ID 时，可按已持久化来源起点/边界键派生；记录 native/derived 与派生规则版本，重启/分页必须得到同一个键。无法判定边界则使用执行级来源区段，不伪造轮次。item key 至少包含项目、execution、attempt、source segment、session、turn、item；不能跨轮复用仅 item_id 的索引。迟到旧 attempt 事件保留在原历史。

## E2：事件与结束语义

至少表达 session、turn.started、message.delta/snapshot/completed、tool 活动、等待输入、turn.completed/failed/cancelled、observation、gap 和结构化执行结果引用；无法映射的来源类型保留 unknown 与原始引用。正常化输出是展示事实，不直接驱动业务完成。

- delta 按原始事件/修订身份幂等追加；snapshot 替换同一 item 的旧修订，不能再次拼接完整正文。没有可靠顺序时保留冲突诊断，不以接收时间覆盖更晚的已知修订。
- message.completed 只完成该消息；turn.completed 不把最后一条消息改为 final_answer。phase 必须来自已验证字段/协议，无法确认则 unknown。
- completed/failed/cancelled 是各自明确的轮次终态；idle、无输出、连接断开、TUI 退出、等待输入不产生终态。终态冲突保留诊断，不能以“成功优先”消去失败证据。
- 终态先于 final 时，保留原轮位置；只有明确 final item 引用或来源正在补传的证据才显示 final_pending。完成对账仍没有 final 时为 final_absent；来源不可达无法确认时为 final_unknown。迟到的真实 final 可以补入原轮，不开启新轮或修改业务状态。
- 登录、信任、权限等宿主等待与 SteerLine Decision 分开。仅观察到文本不能创建可回答的业务问题；已绑定的未解决问题不随 turn 结束自动消失。

## E3：能力与真实来源

能力按 4A 的目标/协议/agent/版本报告独立字段：turn boundaries、message phase、final content、stable item identity、incremental read、history replay、input wait。每项为 verified / unsupported / not_tested，并含证据；structured-read 的声明不证明上述字段全部存在。

| 证据范围 | 规范读模型与展示 |
| --- | --- |
| 轮次和 final 阶段均可靠 | 精确轮次视图，过程与最终回复分离 |
| 有轮次、无 final 阶段 | 按轮展示，明确未识别最终回复；unknown 消息仍可阅读 |
| 仅 terminal/原始文本/结构化结果 | 执行过程与执行摘要；仅在权威执行结算确认后允许执行级折叠 |
| 失联/历史淘汰/source_changed | 保留已有历史，显示 gap/未知；不推断已结束 |

本地 Codex 以实测 JSONL 或公开记录出口映射；不能从 API 文档假定 CLI 含 phase。`--output-schema` 结果的 summary 属于执行摘要；若无独立自然语言 final，不能虚构一条。Orca 按精确 ref 使用公开且已验证的 transcript/worker output 接口，terminal 降级显式标来源；terminal v2 和 supervised 分别验证，不从可用终端推断存在 Dispatch。

已验证的本机 Codex 记录出口为公开 app-server `thread/read` / `thread/turns/list`。Orca terminal v2 可在同一已核验本机执行环境中显式创建并绑定 Codex session，再读取该 session 的公开历史；绑定须保留 Orca 的 Codex 环境及 runtime/terminal/incarnation，不通过工作目录搜索或终端文字猜 session。此证据不证明 terminal 文本本身含 phase，也不覆盖远端或其他 agent。

禁止通过屏幕标题、颜色、关键字、“最后一句”、静默计时或模型自报识别轮次/最终阶段；不读取私有桌面数据库。本机 Orca 的证据不覆盖远端。复用 [Spec 01](../retired/01-orca-remote-protocol.md) 的映射/探测格式，避免维护两套 Orca 输出协议。

精确双链路折叠是目标能力：两条链路均须有原生多轮、phase、结束和重放证据才能宣告达成。上游公开协议缺字段时记录缺口和补齐依赖；经过验证的执行级降级仍可交付，但阶段状态必须标明精确能力未验收，不把降级视为此目标完成。

## E4：持久化、游标与有界读取

Service 的输出采集与 UI 订阅分离：Windows 服务窗口仍开着时，关闭 viewer/TUI 不停止采集；关闭服务窗口会结束服务，重新打开固定入口后从持久 checkpoint 对账恢复，不能把窗口关闭期间描述为持续采集。多个客户端只读持久结果，不能竞争上游读取/Ack。所有后端走同一 normalizer、存储和 Query 契约，原始输出保留；折叠只改变视图。

内容先持久化，再提交包含规范事件、去重身份和来源 cursor 的 checkpoint。文件实现使用版本化追加记录/提交标记，崩溃后只重放完整已提交批次、识别尾部半条记录；内容已存但 checkpoint 未提交时可幂等重放。SQLite 已落地则同事务提交事件/去重/cursor，正文引用先可靠落地再提交。不得先推进 cursor 导致不可恢复的数据丢失，也不得因 UI 查询隐式写业务 runtime。

源 cursor 与 Query cursor 分离：源 cursor 绑定 ref/source/generation，只由采集器使用；Query cursor 绑定项目/execution/attempt/查询过滤与本地逻辑位置，UI 不拿它调用平台。source_changed 开新 segment 并追加 gap，旧 cursor 不复用。重复事件按稳定来源身份去重；缺身份时采用已验证的字节范围/序号边界，若无法保证重放幂等则明确能力不足并保留区段证据，不能仅按相同文本去重而删掉真实重复发言。

ReadConversation、ReadMessageBody、ReadRaw 共享取消和资源上限：事件页默认 200、最多 1000 条且编码正文预算最多 1 MiB；正文分页最多 256 KiB/页并保证 UTF-8 边界；raw 使用 PR 1 契约。超大单条事件改用正文引用，不越过批次预算。快照读取与后台回放分批进行，慢上游或大历史不能阻塞控制命令。实现可下调上限，改变需更新契约与测试。

最终回复完整持久保存，不沿用 activity Detail 的 4096 字符裁剪当作全文；受管正文引用包含版本、摘要、字节数、来源和 completeness（complete/partial/truncated/missing）。读取按正文 revision 分页，更新中不能把两个版本拼成同一正文。终态提交前收到的原始全文与规范正文应可比对；缺页、磁盘满、来源已截断明确报告，不能显示完整。外部绝对路径不得作为客户端任意文件读取入口，远端路径不由本机 os.ReadFile 访问。

本阶段不自动删除规范事件/最终正文；来源历史淘汰仍保留中心已采集内容。磁盘配额/写失败保留上次 checkpoint 并发布可见采集错误，恢复后从原 cursor 对账；源已淘汰部分记 gap。后续若增加保留策略，须留下可读缺失标记。不导入旧 activity 或维持旧 watch 协议格式。

## 验收

| ID | 场景与通过条件 |
| --- | --- |
| E-A1 | 两适配器样本映射同一 DTO；真实来源字段、版本、能力和未验证项均可追溯 |
| E-A2 | 交错 commentary/tool/final、delta 与全文更新无重影；message 完成不关闭 turn |
| E-A3 | 同 session 多轮、跨轮同 item ID、旧 attempt 迟到、来源换代互不串写 |
| E-A4 | 终态先到/late final、无 final、失败/取消、等待授权/业务答案、连接丢失分别准确表达 |
| E-A5 | 内容/checkpoint 每个故障边界重启不丢已提交消息、不重复增量，Query 游标不能跨对象使用 |
| E-A6 | source_changed、重复事件、历史淘汰、部分行/大单条记录有界且缺口可见；真实重复文字不误删 |
| E-A7 | 超过 4096 字符及多页 Unicode final 可完整复原、摘要匹配；缺正文/磁盘满/版本变更不伪完整 |
| E-A8 | 多 UI、Windows 服务窗口保持打开时 viewer 关闭重开、服务窗口关闭后重新启动及离线阅读均不竞争采集；服务停机期间的来源缺口与恢复对账准确 |
| E-A9 | local 与 Orca 各有真实多轮样本及准确能力报告；精确能力缺失时标未验收并给出上游依赖，禁止静默降级通过 |

本阶段不新增终端自由输入和自动宿主授权；不把会话输出当作业务恢复日志的替代。
