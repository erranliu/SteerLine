# 验收：HTTP 服务端命令边界收敛

唯一归属：任务 #92，HTTP 服务端命令边界收敛。2026-10-14 草稿；仅规划，待验收，未纳入执行。

[Spec](../spec/http-command-boundary.spec.md) · [Plan / 测试计划](../spec/http-command-boundary.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划

| ID | 具体行为/最小通过条件 | 方法、环境和安排 | 状态 |
| --- | --- | --- | --- |
| HC92-A1 | 直接调用登记owner并检查handler构造；小端口stub能替代完整Client/SQL/HTTP对象，原重复实现删除 | Plan中的新增TestHTTPCommandBoundary，普通Windows Go；实现时补齐，提取后/PR前执行，结果及结构diff登记本页 | 仅规划，待验收 |
| HC92-A2 | 登记并发同ID/异内容/跨动作；派发计数及原intent、正式消费者竞争、错绑定、rejection、读取故障/unknown与原信封补交核对 | 新增registry stub + 原authority/回执反例；私有临时目录，每次新现场；完整命令见Plan对应行 | 仅规划，待验收 |
| HC92-A3 | 发送原生查看重复/丢结果、原停服请求及边界记录/认证；核验查看副作用一次、原applied、大小/原子性及真实服务丢弃/命令接线 | 既有httpapi与tests/integration定向入口；隔离HTTP/SQLite/app，无真实模型；接线后/PR前及相关最终集成候选 | 仅规划，待验收 |

原始证据保留runner新系统临时目录的summary.json、Go JSON/stderr；结构条件保留实际diff/源码位置。命令/组/场景、时点与能力补齐逐项见Plan测试计划，不能以新增测试的零匹配、skip或编译冒充行为通过。固定L1另记必要回归。

## 人工与阻塞范围

无新增人工项或窗口；#91原人工体验仍归原记录，不阻塞本项。自动失败、共享契约未经确定或关键环境不可用阻止本项交付，不阻碍已独立验证的无关任务。当前不触发任何产品/原生验收。

## 执行记录

尚未执行。实施时逐项记录日期、Spec/Plan固定commit、实际源码commit/dirty、构建/平台、精确命令/真实测试数量、passed/complete或失败、证据绝对目录；保留失败与修正前记录。正式纳入后同步execution-map实际汇总。本页只归属#92，不用兄弟结果替代本项。
