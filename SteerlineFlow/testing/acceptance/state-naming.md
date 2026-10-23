# 候选流程状态字段命名验收

仅归属 [状态命名 Spec](../../spec/state-naming.spec.md)、[Plan](../../plans/state-naming.plan.md) 的普通开发。2026-10-10 完成 Spec 审计修复、工作区实现与最小自动验收，不纳入 roadmap。

| ID | 自动验收项 | 最小环境与范围 | 结果 |
| --- | --- | --- | --- |
| SN-A1 | workflow_stage/workflow_status 贯穿候选重建、查询、API、客户端和 worker_state 事件；删除 finish_task_status；内部步骤及其他状态不误改；无旧别名/兼容解码 | 新建隔离序列化、重建、查询/活动夹具，核对新字段及严格解码 | 通过：新契约、事件→SQLite→查询及真实本地 HTTP 查询 |
| SN-A2 | 准确识别待合并/待输入/阻塞/完成/放弃及关闭后的历史候选；非 await_merge 的 waiting_external 不误判；异步问题及环境/恢复/执行异常保持 | 现有模型/重建及 console 定向夹具，覆盖终态、多候选归属、异步问题与需处理 | 通过：阶段/终态判断、调度槽、候选回滚/唯一交付、反馈及迟到回复 |
| SN-A3 | 新字段标签、活动展示、筛选、候选动作资格正确；Worker/Task 字段及标签、布局、焦点、滚动、缓存保持；受影响构建及静态检查通过 | console 隔离渲染/交互及相关客户端测试，不启动真实模型或安装现场 | 通过：console 全包、客户端/事件预算定向、全项目仅编译、定向 vet/文档/格式 |

新增断言/夹具随实现交付，不扩为完整 Worker 状态重构或全链路验收。未进行真实模型、真实 PR、L2/L3 或 Full；安装环境未修改，未宣称安装现场已更新。

## 审计与修复

1. Spec 补齐公开 WorkflowSummary 与恢复检查范围；恢复 DeliveryStatus 是原派生字段的别名，删除后直接提供流程阶段/状态。
2. 删除笼统“更新格式版本”要求：候选快照没有独立版本，活动 V=1 属通用信封。用既有严格客户端解码及新事件目标字段校验拒绝旧格式，不新增兼容或全局协议。
3. SQLite 可读活动筛选 JSON 路径同步改名，保留同事务、原身份与 1 MiB 正文优先预算；损坏/旧 payload 登记诊断并推进游标，不补画。
4. 消费点保留 Task/候选终态判定；关闭候选残留 waiting_input 不占槽、不制造待答提示，未核实执行仍占槽。丢弃任务仍显示资源需处理，不由旧可选问题复活待答/待合并。
5. HTTP 接线验收初次因测试夹具匹配旧发布提示前缀而失败，第三轮断开后显示原生身份未核实。仅更新 testkit 发布前缀以匹配当前生产模板；独立重跑通过，未修改生产发布/状态机或增加旧模板兜底。

## 实际验证与证据

源码基线 HEAD 为 `872dba4462a4f659d7685eb2a397211a1653c632`，验证包含本次未提交工作区增量；各 test-feedback summary 保存实际命令与工作区状态。报告根目录为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`，下列目录均有 summary.json 与原始日志。

| 目录 | 实际范围与结果 |
| --- | --- |
| 20261010T102227.100879200Z | model/reconstruction/api/query/client 全包通过，包含新契约、旧事件诊断及原领域步骤；同批 console 暴露终态夹具对丢弃资源提示的过强断言，修正为禁止待答/待合并复活，最终 console 另见下项 |
| 20261010T102806.871934600Z | 最终 console 全包通过，4.428s；终态输入、精确待合并组合及既有布局、异步问题、恢复、历史失败和筛选/动作 |
| 20261010T102227.094196200Z | TestPS1RecentActivityReadsLatestTenFromExactAttempt 通过，6.299s；技术/工具噪声及超大正文不挤掉可读变化 |
| 20261010T102131.089409700Z | Plan 中候选/事件/反馈/视图提交定向组通过，402.265s；含 TestMC5 唯一 Delivery、落选资源与回滚，事件提交及失败回滚后公开查询的新字段断言 |
| 20261010T102611.809564700Z | StartReplayPreservesNativeSavedAnswer、LateReplyCannotModifyTerminalTask、CommandIdempotencyAndDecisionVersionArbitration 三项通过，5.344s |
| 20261010T102620.506004000Z | app 答题身份/异步保留及 console 新命名/待合并/原标签定向通过，6.831s |
| 20261010T102333.347056600Z | 首次 APIWorkerStageQueryProjection 失败记录，发布夹具前缀过期；原日志保留，不计通过证据 |
| 20261010T103017.188228900Z | 修复夹具后独立 TestAPIWorkerStageQueryProjection 通过，80.888s；真实隔离服务、三轮受控原生执行、HTTP Run/Snapshot 新字段与原结果一致 |

静态证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-state-naming-static-7f21c8d533c64df399c5d45f4ce7e2d7/`。全项目 `go test ./... -run '^$'` 仅编译、受影响包 vet、文档本地链接及 diff 检查均通过；仅编译不作为全量测试证据。日志为 compile.log、vet.log、links.log、diff.log。

无新增独立人工项。原始日志保留临时目录，仓库仅登记本摘要。

## 2026-10-10 实现复审与最新主干验证

用户授权复审、修复后提交并提 PR。本轮自行复审全部状态消费点和公开契约，没有新增 Worker 状态范围。工作分支 `codex/workflow-state-naming`，最终验证基于最新主干 `a25eeeddb791f0bb9a9c70fa6208e34d934f6c40` 加本次工作区增量；前述 872dba44 的报告保留为首轮证据，不冒充最新主干结果。

- 修复需处理原因：流程 blocked / Task discarded 且执行 succeeded 时，不能用成功的执行维度作为异常说明。新增 TestStateNamingAttentionUsesTheAbnormalAxis，保留执行 unknown/failed/blocked 的优先级。
- 活动查询验证完整 from/to 的已知字段类型，损坏来源/资源/版本均登记诊断、推进原游标，不被控制台静默丢弃；合法初次空来源仍可读取。新增 TestStateNamingActivityReportsMalformedTransitionFields。
- 同步主干时保留 #286 的阶段变化单消息规则及 #284 的新 Orca 路径；未复活退役的 Orca 测试分支。原受控发布前缀修复已在最新主干存在，本 PR 不重复修改该夹具。
- 最新主干删除旧适配器导致 8 个现有本地导航失效；逐项核对 872dba44 的 Git 对象，将链接改到该固定历史提交并标注历史，不改变原历史事实或恢复旧代码。

| 报告目录（同上根目录） | 最终复审结果 |
| --- | --- |
| 20261010T104107.946857300Z | console/model/reconstruction/api/query/client 六包通过，4.137s，含新增异常维度/损坏字段诊断及当前主干流程变化单消息回归 |
| 20261010T103940.774197000Z | SQLite RecentActivity 原身份/预算定向通过，5.748s |
| 20261010T103940.901584800Z | 精确选择事件提交回滚、MC5 唯一交付、反馈持久化/过期 actor、原答案重放、终态迟到回复、视图只读及原会话流程，共七项通过，134.531s |
| 20261010T104019.098232900Z | app 原问答身份/异步保留与真实本地 HTTP 三轮查询通过，82.561s |

静态证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-state-naming-review-8197ac634e584958abaa201f93aab8fd/`。全项目仅编译及受影响包 vet 通过；修正历史链接后最终文档检查为 333 文件、2002 本地导航、0 错误，diff 格式检查通过。最终文档日志 final-links.log / final-diff.log；初次发现失效链接的 links.log 保留。未运行 full、真实模型或安装版验收。
