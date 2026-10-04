# 三线解耦共同实现约束

适用于 #73–#82 的独立 Spec/Plan；目标 `TASK-EXECUTION-CONSOLE-DECOUPLING`。本文件不另占任务 ID，不是一项独立交付。各 Spec 的范围、前置与验收仍须分别满足。

## 最终依赖与所有权

| 消费者 | 允许项目内直接依赖 | 禁止依赖/职责 |
| --- | --- | --- |
| task/model | 自有纯值/规则，原则上仅标准库 | 执行模型/后端、应用、产品、存储、宿主 |
| execution/backend | 标准库 | 业务模型/用例、SQL、具体实现 |
| execution/model | execution/backend、自有纯规则 | task/model、业务用例、adapter、SQL、产品、宿主 |
| product/api | 标准库与必要稳定纯标量 | Domain/Runtime、完整持久记录、业务/客户端/SQL 实现 |
| storage/ports | 各域纯模型、backend 纯值、公开纯契约 | product/query、业务用例、SQL/组装/宿主实现 |
| task | task/model、自有小端口、必要 storage/ports | execution model/backend/应用、具体 SQL/adapter、产品实现、app/console |
| execution | execution model/backend、任务小端口/task/model、必要 storage/ports | task 应用实现、具体 SQL/adapter、app/console |
| product/query | 纯模型、api、读取端口、纯投影 | task/execution 应用、具体 SQL/adapter、app |
| product/client | api、注入的读取/命令/连接小端口 | 业务/SQL/adapter/app 实现 |
| console | product/api、独立 UI 状态与偏好 | 内部模型/持久记录、业务/SQL/adapter/app/client 实现和默认产品 I/O |
| execution/adapters | backend 纯契约、必要纯值、执行侧协议与资源小包 | 业务用例、存储写端口、app/UI |
| execution/workspace、publication、process | 自有副作用小契约、必要 backend 纯值 | 业务状态机/写端口、app/UI |
| storage/sqlite | ports、纯模型/值、product/query 纯投影 | 业务用例、产品读取/命令 I/O、adapter、app/host |
| app/host | 注入的调度/控制/元数据小接口 | 具体 SQL/adapter、业务资格/派发/交付/恢复状态机 |
| app | 配置、具体构造与各域端口 | 业务状态转换、第二目录注册表/写者 |

storage/ports 可引用多个领域纯值；task 对该纯端口包的间接依赖不等于获执行业务权限。仍检查 task 实际消费的方法签名、转换声明和结果，不允许携带 Workflow/Execution、PR 或 session 记录。product/query 的共享纯投影无 I/O；storage/ports 不导入 query，防止读取接口成环。

生产不得依赖 testkit、tests、runner 或 fixture。检测覆盖全 OS/tag 的生产/测试源码及 Windows/Linux 生产直接和传递依赖，不用包改名、生成文件排除或新增白名单绕过。

## 类型拆分与中间交付

task/model 拥有固定任务输入、图、资格及规范结果；execution/model 拥有执行记录和 PR/资源事实；backend 拥有实现交互纯值；api 拥有用户查询/命令/回执；ports 拥有原子提交声明及纯读取契约。每类型一个定义所有者，不把 contracts 改名为新的共享聚合。含任务输入的 worker 提示组装归执行应用，不进入 execution/model。

#73 先建立能独立消费的纯定义和端口；宽产品快照由 #77 消除，业务整包读取由 #79 消除，宿主剩余类型由 #80 消除。中间尚未迁的唯一定义可在原包存留，并列出后继所有者；迁出定义和调用路径立即删除，不留 alias/转发。最终依赖矩阵按各阶段负责边界逐步启用，不能要求 #73 提前完成后续全部用例迁移，也不能拖到 #82 才恢复守卫。

## 跨域提交

任务结果仅含任务/契约/领取身份、期望版本、唯一结果关联和经过执行核验的结论，不把 PR 证据放进任务需解码的不透明载荷。结果应用小端口只向执行用例注入，产品/后端无此写能力；原子提交须关联已核验的唯一交付记录，任意 success 布尔值不是关闭授权。merge commit 等后继启动基线属于执行事实；任务资格只据规范交付事实，后继启动由执行查询结果关联取得固定 base。

首次领取和创建执行记录、唯一交付和任务关闭/DAG 修订仍各自一次 SQLite 事务。任务先生成只含任务变更的声明，执行生成执行事实/版本声明，存储的专用提交边界组合并重检原前值；任务调用者不因此取得写执行记录的句柄或任意事务回调。重复原结果返回原回执，异内容或旧版本拒绝，事务失败不公布候选成功状态。

Task 一次领取；后续候选沿原领取/base/固定契约新增 WorkflowRun。参与数仅开始时达到五个拒绝，允许竞争偶尔超出；放弃、停止、完成分别表达。多个合并/未知/迟到事实不覆盖唯一交付；新增候选、丢弃及交付按原任务/集合版本仲裁。丢弃意图归任务，执行核验与停止/资源清理归执行。

## 服务、来源与并行

actor 与业务异步单步通道归 execution；host 只驱动注入端口并承载控制传输/心跳/锁。先 intent、一个副作用、提交原绑定/receipt，再下一步/ack；事件/正文/cursor 同事务、心跳不夹带业务。未知结果保持原 Operation/request/身份只读对账，重启不解除人工门槛或新建替代会话。

任务候选的精确 WorkflowRun/Execution/session 与规划来源分别定位；规划实际身份由后续规划任务交付，不伪造 worker 身份。未来 WinUI 经可序列化产品协议接入，Go client 仅供 Go 消费者；本轮不实现 planning、HTTP/SSE 或新后端。

#73 可与 #38 并行，产品定义/import/构造共享段由 #73 顺序集成；#74 交付后 #75/#76 分线，#77 另以 #38 为硬前置。#39 不作为重构硬前置，可用隔离固定构建/现场验收；迁移后的构建按实际行为/接线变化定向重验。第一阶段及 #39 只验本机；存量远端代码的迁移/编译/拒绝保护保留，不开展新远端流程或要求真实远端证据，也不以本机结果声称远端已认证。

#74 必须提交实际 Service/CommandService/Config 字段/方法/文件清单，并先拆后继共同改动段；#75–#79 只改自己所有者文件，公共类型由定义所有者先集成，app/cmd/runner 共用段指定单个集成所有者。#80 的最终组装不替前置补编译或调用者迁移。其他既定产品任务不隐含等待全部重构。

## 最小验证

各 Plan 选择定向代表及一次 L1；最终 #82 补 delivery.basic 和 recovery.waiting-merge-restart。迁移旧/新 parent/subtest/关键断言与命令同次映射，run/pass 非零且无 skip 才通过。原生实际启动/权限/协议变化才补相应最小证据，fake 不填能力缺口。不默认跑 Full、完整 L2 或全部 live；新增场景/环境能力按 E2E README 在具体 Plan 中登记。
