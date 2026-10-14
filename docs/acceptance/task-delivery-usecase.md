# 验收：任务最终交付用例独立

唯一归属：任务 #95，任务最终交付用例独立。2026-10-14 草稿；仅规划，待验收，未纳入执行。

[Spec](../spec/task-delivery-usecase.spec.md) · [Plan / 测试计划](../spec/task-delivery-usecase.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划

| ID | 行为与通过条件 | 方法、环境/时点 | 状态 |
| --- | --- | --- | --- |
| TD95-A1 | 小读取/provider/声明/提交端口完成用例；无完整Config/CommandStore/worker控制，旧规则单一，直接/传递闭包隔离 | 实施新增delivery包TestDeliveryUseCase与TestDeliveryBoundary；stub、Windows/Linux结构，提取后/PR前 | 仅规划，待验收 |
| TD95-A2 | 原冻结provider/候选/任务/仓库/base/head与merge证据逐项拒绝反例；unknown publish合法merged可交付，非merged/替代PR不交付 | 用例stub+原正式execution入口始终验；最终基线含#94时加TestDevelopmentUnknownPublicationMergedDelivery，隔离SQLite/provider，命令见Plan | 仅规划，待验收 |
| TD95-A3 | 两候选/版本竞态仅一个Delivery，原重试回执；事务失败无部分Task/WorkflowRun/Delivery/图更新；丢弃迟到合并只审计，清理问题不重开任务 | 实施新增TestDeliveryAtomicCommit与原ResultCAS始终验；最终基线含#96时加TestWorkerRuntimeDeliveredCleanup，真实SQLitefixture，提交接线后/PR前 | 仅规划，待验收 |
| TD95-A4 | 正式创建/领取、受控真实Git/PR事实到唯一交付/后继就绪；待合并重启原身份保持 | 已注册L3 controlled delivery.basic、recovery.waiting-merge-restart；各隔离fixture，PR前及最终共享提交候选，两个scenario通过且清理完整 | 仅规划，待验收 |

新增测试在Plan实施步骤补齐；所有验收项的命令、准备/时点与证据映射见Plan测试表。runner原产物放新的系统临时目录，E2E保留scenario身份/回执与清理事实。本项没有真实GitHub发布要求，controlled不宣称native已验。

组合证据按TD95-A2/A3登记实际最终候选基线commit及已包含的#94/#96；未包含时明确记条件未触发，不记通过、不等待。包含却缺入口/零匹配失败；组合不能由固定L1代替，不以兄弟验收记录替代本项最终候选结果。

## 人工与执行记录

无新增人工窗口。关键自动失败、provider/原身份未知不可标为人工后置；资源收尾失败按原事实登记，不伪造外部资源已退出。

尚未执行。按ID登记Spec/Plan版本、源码/dirty/构建、精确场景/命令/数量、通过或失败及summary/原日志/结构diff绝对临时路径，保留所有失败。固定L1另记回归；正式纳入后同步execution-map。本页只归属#95。
