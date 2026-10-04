# Spec：数据库活动与操作回执收口

依据：用户要求执行精简建议 3、4 并提新 PR。基线 main 886a520（#205 已合入）。

1. 中心活动仅随原业务事务写入数据库，移除 activity.jsonl 镜像、文件 seq 恢复、writer 缓存/关闭及晚到活动的文件写入。原始日志、会话 source/mapper、正文、文件采集游标/预算、数据库活动去重与原 Run/Execution/Attempt 归属保持。产品 Activity 以数据库 EventID 排序；服务控制台输出保留。
2. 操作准备/请求构造、结果公共状态与启动回执身份检查合并为少量具体函数，不新增框架、状态机或第二写者。每步仍是持久 intent → 原请求 Execute/只读 Reconcile → actor 校验 → receipt/附件原子提交 → 下一动作。
3. question reply、live feedback、session、turn、ack、stop/cleanup 的接收条件与业务转换独立保留；accepted 不等于完成，未知输入不重发，旧身份/人工恢复/冻结交付证据与清理独立回执保持。原 stop 格式和确认进程缺席路径保留，不为减少行数重设计外部协议。
4. 无持久结构变更：FixedExecutionProfile v6、SQLite 12/runtime 17/PolicyResources v3 保持；不读写、迁移或重建旧 activity 镜像。

验收：数据库活动跨 reopen 归属/去重、无镜像依赖/写入、旧 Attempt 晚到活动不串 run、原始取证不受影响；共同请求拒绝改 payload、所有动作仍遵守提交屏障、未知/拒绝/提交失败与回执原身份；现行交付/重启闭环和清理成立。
