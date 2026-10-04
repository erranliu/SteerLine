# 数据库活动与操作回执收口验证

行为依据：[Spec](../../spec/activity-db-receipt-simplification.spec.md)、[Plan](../../spec/activity-db-receipt-simplification.plan.md)。基线 main 886a520（#205），未修改候选规格或路线图。

## 行数

主包/全仓生产 Go 均净减 285 行；测试 Go 净减 122 行；全仓 Go 含测试净减 407 行。计入新增 lifecycle_receipt helper 和数据库活动回归；逐文件见 [source-summary.json](source-summary.json)。统计物理 Go 行，生产排除 _test.go，主包不含子包。

## 行为与断言映射

- 删除 ActivityWriter、尾行修复/文件 seq/reopen 去重、服务 writer 缓存/关闭、晚到活动文件写入。活动仅随原事务写入 SQLite，普通与旧 Attempt 事件保持原 run/execution/attempt。服务控制台 JSON seq 保留，产品 Activity 使用数据库 EventID；原始 events/history/source/mapper 不改。
- 原镜像文件专属测试删除；数据库回归实际证明旧镜像路径被目录占用仍可提交、服务无镜像错误、跨 reopen 去重、原活动身份/payload 与原始取证字节保持。原完整正文/游标重放与当前客户端无 raw fallback 用例保留。
- new/originalLifecycleOperation 合并已提交操作查找与原身份检查；checkedLifecycleRequest 合并原 payload/摘要和请求检查。fresh intent 仍需 commit 成功才返回，旧外部意图仅 Reconcile。动作自行决定 confirmed/rejected/accepted 条件，保留各自 key、kind 和原请求。
- lifecycleOutcome 只归一化外部 status/diagnostic；question、live feedback、session、turn、ack、stop 仍分别处理接收、拒绝、未知与业务转换。checkedWorkerTurnReceipt 复用 session/turn 的原 Action/Execution/Attempt/session generation/config/resource/Command 检查；observer、附件和进程处理仍归动作自身。
- session receipt 原子提交后才创建 turn intent，question 原生路由独立于 live feedback，ack 不代替确认，旧 stop 格式/进程缺席确认、交付冻结与原资源 cleanup receipts 保留。未知输入不重发、晚到结果不改变 terminal task。

未升级数据结构：Activity v1、FixedExecutionProfile v6、SQLite 12/runtime 17/PolicyResources v3 保持。旧镜像不读取、迁移或重建，没有第二状态机/写者或新的诊断导出框架。

## 最小验证

最终报告全部 complete/pass，精确命令/命中和报告 SHA-256 见 [validation.json](validation.json)，完整日志保留 bin/activity-db-receipt-simplification/。

- 数据库活动、完整会话重放、当前客户端规范 Activity/Attempt 归属与渲染定向通过。
- 操作/回执 39 parent/subtest：原 intent/提交屏障、receipt/附件/CAS/提交失败、原 binding/stop unknown、session/turn 接收、原恢复和旧 Attempt/terminal late receipt、冻结清理证据通过。
- L1 quick 15 包、215 parent/subtest 全通过；包含本机双轮、终端原身份/重启及 dispatch/reply/send/ack/task 丢失回执的现行 Service 集成；all tags 与跨平台/依赖守卫未放宽。
- L3 controlled 的 delivery.basic、recovery.waiting-merge-restart 通过并完成清理；真实 Git/loopback PR、原身份重启、不重发及唯一 TaskDelivery 保持。
- 40 包编译、Windows/Linux amd64 CLI 构建、diff --check 和消费者清单通过。

未选 Full、完整 L2 或 live 模型：本次删除诊断副本和复用现有处理，原生协议与权限未变；现行确定性 Service 集成、定向提交/恢复边界和受控闭环覆盖本次影响。最初缺少已删除文件 IO 的 import 清理导致的编译诊断留在 activity 目录，最终通过证据使用 activity-final。
