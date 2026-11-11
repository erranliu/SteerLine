# 执行观察与结果

本文统一维护公开活动、最近消息、会话 final 与正文阅读。SteerLine 保存候选/Runner Run 的固定关联和公开事件副本，由产品查询生成投影；原生采集、会话与来源核验归 Runner。查询不连接 Codex app-server、读取 Orca 私有终端或扫描原生日志，不推进执行或授予恢复资格。

当前规则以所选 Runner 公开能力及专属验收为准；完整工具 transcript、私有日志重放和第二套原生历史 checkpoint 均不属于调用方能力。问题与输入见[反馈设计](decisions.md)，接口见[本机协议](local-client-protocol.md)。

## 信息含义与来源

| 信息 | 当前含义 |
| --- | --- |
| 流程活动 | 已保存的公开变化，保留事件来源与顺序，不证明交付或资源停止 |
| 最近消息 | 最佳努力的只读提示，不是完整会话、心跳或最终结果 |
| verified final | 原轮次来源已核实的最终正文；正文存在仍不证明测试或 PR 成功 |

公开结果的成功、失败、取消、unknown 和 unreported 按原义展示。没有合格 final 时保留状态与缺口，不用最后消息、工具输出、初始化回复或自然语言成功替代；补读、补报及执行推进归 Runner。

## 流程与状态活动

workflow_changed 的 transition.from/to 投影为 ActivityEvent v1 的 worker_state，区分“Workflow 开始”“Workflow 切换”“状态切换”和“诊断信息更新”。节点保留英文标识，状态中文显示；开始/切换包含目标状态，单独状态/诊断变化另标所在 Workflow。detail 保留原诊断，meta.from/to 使用 workflow_stage/workflow_status；缺字段报告诊断，不以当前状态或日志补画历史，无 transition 的事件归内部详情。

原 event_id、Run/Attempt、revision 和 created_at 保持来源；ActivityEvent 的 UTC ts、source、run_id/task_id、item_id、kind/status、title/detail、elapsed_ms 和 meta 按公开契约投影。数据库 seq 决定顺序、去重和游标，不用文件序号或显示时间。绑定与事件在同一只读事务核验，历史 Attempt 必须有原公开事件依据；重复读取不换归属，冲突不覆盖原结果。

近期查询取原候选/Attempt 最近五条流程变化和十条内部事件，共享 1 MiB 输出预算。API 按 seq 升序返回，Web 降序显示，内部事件折叠；无可读变化显示空状态，不从技术名推定业务节点。流式 cursor 不与近期查询混用；准备等待 execution_wait 对象/null 语义归[本机协议](local-client-protocol.md#查询路由)。

来源：[活动读取](../../internal/storage/sqlite/activity_view.go)、[查询与预算](../../internal/product/query/activity.go)、[变化投影](../../internal/product/query/runner_activity.go)。

## 最后观察到的消息

只调用固定 Runner EXE 的公开 last-message，采样方式及后端支持归 Runner。应用核对候选、Run、当前 Attempt、state-dir 以及返回消息的 session/turn；message 可为 null，只接受 assistant/tool、最多 4,096 字节及有效 observed_at，见[消费端校验](../../internal/app/runner_last_message.go)。查询不创建会话、发模型请求或推进观察时间，不支持的路径不从其他来源补造。

Web 只为当前选中且在线、页面可见的执行每三秒查询；query key 绑定 home/project/instance/candidate/Run/Attempt。切换/隐藏时取消旧请求，迟到响应不覆盖新目标；Runner 正常观察不随页面切换停止。摘要显示“观察于”本地时间和距今时长，查询刷新时间另列，见[最近消息组件](../../web/src/features/runs/last-message.tsx)。

失败保留最后样本并显示诊断，无样本显示暂无。长时间无消息不推断卡死、失败或恢复资格，预览不替代 verified final、完整工具过程或任务结果。

## 公开结果来源

活动独立页使用[窄索引和单段读取](../../internal/product/query/activity_reading.go)，不复用原 final 客户端的自动追页路径。原持久记录的拥有者在同一事务维护正文引用、摘要和字节长度，正文仍在原记录；GET 不收集原生日志。首次显式展开允许验证该条完整正文一次，后续段复用有总量/生命周期上限的已验证内存缓存；命中仍核对原归属和引用版本。历史输入缺轮次时明确显示未记录，完整原生 transcript 不在本期范围。

候选原观察链可非阻塞通知[消息观察器](../../internal/app/message_observer.go)，每绑定后台最多 10 秒尝试一次，与执行页 3 秒按需读取共享在途/尝试时间，应用内并发最多 2，公开读取预算 1 秒。失败、null 和 tool 均消耗间隔，只有合法 assistant 替换原样本；样本按原绑定持久化但不递增业务 revision，新 Attempt/session 不沿用旧预览。活动 GET 只读本地样本，关闭 UI 不影响原执行或后台观察。终态停采样，正文/问题缺口不从旧轮次或其他来源补造。

final 索引只接受公开事件中 verified、正文非空且来源完整的 final，核对候选、Run、Attempt、session、turn、source 和 item/event 身份。SteerLine 不以其他轮次或候选的正文满足原请求。

SQLite 返回原事件与正文事实，查询层生成 DTO、正文引用和分页元数据。final 引用固定原 Run/Attempt、事件/来源、revision、SHA-256 和字节长度；读取重新核对完整正文与引用。存储不生成页面能力、协议或游标，查询不直接读取 Runner 私有数据。

## 存储与阅读

| 读取 | 当前边界 |
| --- | --- |
| 会话事件页 | 默认 200、最多 1,000 条，共享 1 MiB 序列化预算；游标绑定项目、筛选条件和事件位置，失效不拼接旧页 |
| 完整正文 | 最大 16 MiB；缺失、损坏、超限或截断不得报告完整 |
| 正文分页 | 每页最多 256 KiB，按 UTF-8 安全边界切分；旧引用、摘要或长度不符拒绝 |

正文预算与分页来源见[会话查询](../../internal/product/query/conversation.go)及[分页常量](../../internal/product/query/events.go)。HTTP final 直接返回预算及分段路径由[本机协议](local-client-protocol.md)维护，不与完整正文上限混同。

## 阅读与诊断

Web 按需读取完整及历史 final，异步响应保持原项目、候选、Attempt、turn 和正文身份；失败区分旧缓存与当前可核实内容。缺口、空结果和有界预览不显示为完整回复。完整原生过程由[原会话查看入口](../integration/steerrunner.md#配置与能力要求)定位原工具，不由活动摘要代替。

活动阅读页的后台消息采样使用项目生命周期，不继承单次观察 tick 的取消上下文；项目结束先取消并等待该项目采样退出，再关闭原存储。采样失败仍消耗尝试间隔，正文及交互索引查询不触发采样。环境摘要只投影必要轮次事实，原请求、final 和报告正文由显式阅读接口获取。

查询、刷新和浏览器 journal 不重发业务输入、不创建会话或清除诊断。旧 TUI 布局/快捷键及 run/console CLI 已退役，见[退役断言](../../cmd/steerline/ps3_cli_test.go)；当前操作归[Web 指南](../../web/README.md)。不读写、修复或迁移旧 activity.jsonl，也不以原生日志填补规范缺口。ActivityVersion 与存储/checkpoint 版本独立，控制台输出序号不是业务游标。

## 验证与历史

活动的实现范围见[Plan](../../SteerlineFlow/plans/execution-observability.plan.md)和[专属验收](../../SteerlineFlow/testing/acceptance/execution-observability.md)。会话来源和各后端受测范围归[第四批历史入口](../../SteerlineFlow/archive/design/runner-reliability-history.md#第四批整理来源)，旧采样与 CLI 说明见[第六批记录](../../SteerlineFlow/archive/design/docs-sixth-batch-review.md)。合并前两份原文与当前入口见[合并来源](../../SteerlineFlow/archive/design/runner-reliability-history.md#首轮文档合并)；历史通过不扩大当前构建能力。
