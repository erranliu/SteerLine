# Spec SQLite写入实现归包

任务 ID：70；任务名称：SQLite写入实现归包；目标：`CORE-IMPLEMENTATION-DECOUPLING`。配套 [Plan](core-decoupling-09-sqlite.plan.md)。硬前置：#69 [中心写入端口收窄](core-decoupling-08-write-ports.spec.md)整体交付；一个实现 PR。

## 目标与范围

遵守[后续维护共同约束](core-maintenance-constraints.md)。将当前中心的 SQLite 连接、schema、SQL 读写、声明提交和事务执行归存储模块，中心以 #69 的稳定端口消费。复用现有 internal/steerline/sqlite 的只读实现；不新增第二个 Reader 或一套镜像数据库模型。

迁移输入是 #69 后的 SQLiteStore、schema/read/write/checkpoint、worker/attachment/execution/conversation 存储、任务/DAG/交付 SQL 以及仍承担 DB 格式校验的 helper。连接/文件锁的数据库责任归存储，服务进程承载及业务恢复规则仍归原所有者。

## 必须交付

1. 存储包实现 #69 的全部消费端口及既有 query Reader；只依赖 stdlib、SQL 驱动和纯值/规则/投影包，不导入 steerline、steerlineapp、客户端或执行实现。SQL 中的关系、固定身份、格式、CAS/唯一约束校验保持；业务用例和外部 I/O 不搬到存储。
2. 整体迁移连接创建/只读打开/关闭、schema 创建与当前版本拒绝、事务/锁/连接限制、WAL 与同一读快照语义；业务用例通过端口调用，外层组装持有具体连接并显式注入。只读入口不得建库/建表/迁移/消费命令。
3. worker 业务记录、附件、回执和 checkpoint 保持原事务；任务批次/领取/交付保持原事务；会话内容/source cursor 独立事务；heartbeat 只写服务元数据。CAS、唯一 TaskDelivery、同内容幂等与异内容冲突在数据库边界重检。
4. 错误以纯契约中的类型/码返回，保留 no-row、格式不支持、身份冲突、版本过期、锁/提交失败的可区别语义；不得因搬包把故障伪装为记录不存在。应用层才决定展示和业务处理。
5. 当场删除中心原 SQL、具体 store/schema 和无消费者的转发，迁移存储单元测试，保留通过真实应用构造的集成验证。不导出任意 SQL 或测试写入口。连接关闭及 Windows 文件占用行为按现有生产语义验证。

## 并行与后继

可与客户端线并行，存储包及组装连接段由本项修改；产品客户端只消费应用查询/命令端口，不能 import 存储具体类型。若客户端默认实现仍具体开库，由本项与 #71 在外层组装改为注入，契约保持，集成构造改动顺序提交。不得要求 #71/#72 再做一次存储搬迁。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| CM70-1 | 全部 SQL/连接/schema 实现归唯一存储模块，中心业务消费者只用 #69 端口；存储没有中心/组装/平台反向依赖 |
| CM70-2 | 实际 SQLite 代表用例保持原子回滚、CAS/幂等/唯一交付、会话 cursor 及固定 WAL 查询；只读前后无文件/业务写入，连接关闭可释放资源 |
| CM70-3 | 产品实际构造及最小闭环通过，#67 守卫和查询 DTO 语义保留，测试映射及模块文档反映新归属 |

仅运行迁移包和一组必要应用接线，复用 #69 已有代表断言，不新建重复矩阵或旧版迁移。
