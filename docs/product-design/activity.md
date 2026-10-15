# Activity 契约：公开 Runner 事件与 Web 执行详情

SteerRunner 独占执行状态写入。SteerLine 在自己的数据库保存固定候选与 Run 的关联、公开事件和当前投影；Web 通过正式产品查询读取这些事实，不打开 Runner 数据库、Codex 私有数据库或 rollout 文件。实现与验证见 [Plan](../plans/execution-observability.plan.md) / [专属验收](../acceptance/execution-observability.md)。

## 流程与状态活动

Runner 在提交 Run 的同一事务里比较真实 workflow stage、status 和 diagnostic。发生变化时追加 `workflow_changed`，记录 `transition.from/to`，保留原 event_id、Run/Attempt、revision 和 created_at。初次记录 from 为空。技术 operation 的接受或提交不单独代表业务流程改变；消息摘要也不参与结算或恢复。

SteerLine 把 transition 投影为 ActivityEvent v1 的 `worker_state`：title 使用可读的流程/状态变化，detail 保留原诊断，meta.from/to 使用 `workflow_stage/workflow_status`。缺少目标字段的规范变化报告采集诊断，不用旧字段、当前投影或日志补画历史。已存在但没有 transition 的 Runner 事件仅作为内部执行详情展示。

ActivityEvent 的 UTC ts、source、run_id/task_id、item_id、kind/status、title/detail、elapsed_ms 和 meta 保持既有契约。数据库 seq/EventID 决定顺序、去重和游标，不使用文件序号。正式读取在同一事务核验项目/候选/Run/Attempt；历史 Attempt 必须有原公开事件作为身份依据。

RecentActivity 按固定候选和 Attempt 取最近五条流程/状态/诊断变化、十条内部事件，合并后按原 seq 排序，共用 1 MiB 输出预算。Web 默认显示可读变化，技术事件放进“内部执行事件”折叠详情。没有可读变化时如实显示空状态，不把技术名翻译成不存在的业务节点。最终回复通过独立的原 final/正文契约读取，不由活动摘要代替。

## 最后观察到的消息

最近消息是独立的最佳努力活动提示，不是完整会话、业务终态或心跳。Runner 复用本地 Codex 既有 app-server 通知流，采样 completed agentMessage 和固定工具开始/结束标签；不读取 reasoning、用户输入、命令、工具参数或输出。通过原 Attempt/Generation/Session/Turn 守卫并脱敏后，仅在进程内保存每 Run 一条有界摘要，不写入 Run revision 或业务事件；乱序或过期样本丢弃且不阻断原观察。服务重启后为空，Orca 当前无该采样能力。

Runner 的公开 `last-message --run-id` 只读该缓存，返回原 Run/Attempt 和可空 message。摘要含来源 Session/Turn、kind、text 和通知到达的 UTC `observed_at`。不启动服务、不重新打开原生会话、不发模型请求、不扫描所有会话或回读私有日志。重复查询不会推进 observed_at。

SteerLine 路由 `/runs/<candidate>/last-message?execution_id=<Runner Run>&attempt_id=<Attempt>` 在 HTTP 和应用层核验当前固定目标，随后只调用已固定 EXE 的公开 CLI；返回不同 Run/Attempt 或不合格消息时拒绝。

Web 在当前选中执行详情展示一条摘要、明确的“观察于”本地时间和距今时长，独立于近期活动/最终回复 Tab。仅该组件在线且 document 可见时每三秒查询；切换候选/Attempt、离开执行页或浏览器后台会停止并取消请求。query key 固定 home/project/instance/candidate/Run/Attempt，晚到旧响应不会覆盖新目标。不为其他候选建立消息轮询；Runner 的正常执行观察不因页面切换停止。

读取错误保留最后样本并显示失败；没有样本显示暂无。长时间没有消息不推断卡死、失败或恢复资格。活动/最终回复工具栏的“查询刷新于”只表示查询完成，不能冒充消息或业务变化时间。

## 持久性与诊断边界

中心不读取、写入、修复或迁移旧 activity.jsonl，也没有文件 seq 恢复、镜像或 raw fallback。完整 final 的正文与来源继续归公开 final/会话查询；本次临时摘要不替代它们。底层原生日志仅供独立人工诊断，不作为 UI 或执行恢复权威。

`steerline run -console pretty|summary|json|off` 的服务输出策略保持；json 控制台 seq 只表示当前输出流顺序。ActivityVersion 由 payload 契约决定，不随存储或 checkpoint 版本联动。本次新增公开 Runner transition 和独立消息 DTO，不新增另一套可推进执行状态机。
