# Plan：多候选会话与唯一PR交付

任务 ID：37；任务名称：多候选会话与唯一PR交付。唯一执行依据：[Spec](candidate-workers.spec.md)。硬前置：#36，一个独立实现 PR 交付。

1. 按当前源码盘点Task单运行假设，复用WorkflowRun、Execution、Query/Command、SQLite及现有异步单步副作用通道；不另建候选Service/引擎或single/multiple模式。保持worker-only发布、独立环境blocker和产品只读规范数据库，不恢复已删旧路径。
2. 接首个领取和人工start_candidate，各自冻结执行配置、共同base、独立原会话与工作区。开始时读取参与数，已达5个拒绝；不增加数量预留/CAS/槽位/队列，保留普通业务版本与幂等。
3. 接人工abandon_candidate：核验已知PR未合并与前值，退出实验回执与停止Operation分开；复用原后台动作和actor结果交接。慢/未知停止不阻塞新实验，晚到启动/反馈结果保存原事实但不重新授权，重启不复活。
4. 查询/反馈/正文与独立blocker精确绑定候选，环境动作保持原版本/服务实例，新反馈阻断时拒绝且不补投。活动与正文只读规范数据库，不尾随/回填raw日志；提供实际配置、状态、耗时与结果定位。
5. 接包含放弃历史的PR集合核验和唯一Delivery；处理新增/放弃/交付竞争、多合并和迟到事实，区别单候选放弃与整体Task丢弃。
6. 迁移完整消费者与代表测试，删旧单运行假设和重复实现；向 #38/#39交接真实查询/命令与中性夹具。实现后同步实际路径和版本，不修改已创建任务的固定契约或运行时DAG。

## 测试计划

下面TestMC1–TestMC9是本任务规划新增或迁入的顶层用例，当前不是已存在入口。优先复用已有单候选、事务、会话和恢复断言；实施时只补缺口，记录旧到新映射。若前置解耦已经改变包路径，开工Plan按实际唯一所有者更新命令，不保留第二测试路径。

| 级别/组或场景ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向Go：MC1、MC2、MC7、MC9 | Windows；临时SQLite、Git和中性后端；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/identity-count -- ./internal/steerlineintegration -run '^TestMC[1279]$' -count=1` | 身份、开始检查和格式路径接通后 | 实际命中各ID及子测试；领取一次、串行第6次拒绝、放弃后再增；不要求数量并发严格不超额 | 对应输出目录的summary/Go JSON与固定身份记录 |
| 定向Go：MC3–MC6、MC8 | 隔离真实服务/SQLite，可控来源与PR provider；无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-workers/flow-abandon -- ./internal/steerlineintegration -run '^TestMC[34568]$' -count=1` | 反馈、放弃与唯一交付接通后 | 原身份/请求、退出实验与真实停止区分、合并后拒绝、回滚及迟到事实正确；无fail/skip或零匹配 | 原始事件、PR/Delivery/停止回执与测试JSON |
| L1固定最小入口 | 本次隔离STEERLINE_HOME，无原生开关 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-workers/l1` | 定向稳定、自查完成后一次 | 守卫/原单候选代表命中且整体通过，记录源码及构建身份 | l1/summary.json与原始日志 |

新用例复用当前隔离环境与worker侧发布fixture，不使用已退役host_service发布或旧DeferredDecision/Feedback。MC3覆盖一个blocker下拒绝/解除不补投与兄弟继续；MC4核对worker发布和raw日志不参与产品读取；MC8用受控慢/unsupported/unknown停止及晚到启动/反馈结果验证原回执屏障。复用既有断言，只补候选归属缺口，不增加全套单worker故障矩阵。证据目录每次使用新的空目录，重跑不得覆盖旧证据。数量边界不启动五个真实模型；不选择Full/L2/L3或新的权限认证矩阵，真实后端增量归 #39。
