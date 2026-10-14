# 验收：worker 运行时资源所有权收敛

唯一归属：任务 #96，worker 运行时资源所有权收敛。2026-10-14 草稿；仅规划，待验收，未纳入执行。

[Spec](../spec/worker-runtime-ownership.spec.md) · [Plan / 测试计划](../spec/worker-runtime-ownership.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划

| ID | 行为与通过条件 | 方法、环境/时点 | 状态 |
| --- | --- | --- | --- |
| WR96-A1 | 小端口即可验证runtime，单一资源/关闭owner；无Service/RunView/apply业务闭包或数据库/上层能力，旧资源状态移除 | 实施新增workerruntime包TestWorkerRuntimeOwnership与TestWorkerRuntimeBoundary；Windows/Linux源码闭包，提取后/PR前 | 仅规划，待验收 |
| WR96-A2 | 真实原身份、在途互斥/一次Execute、unknown仅Reconcile、迟到/旧Attempt/generation隔离；owned/attached、关闭取消/原停服策略，无资源残留 | runtime小端口与原停服/迟到/工作区checkpoint fixture；Windows受控进程，资源接线后/PR前，命令见Plan | 仅规划，待验收 |
| WR96-A3 | 成功确认遵守两道actor屏障；交接前取消runtime报error，入队后迟到/失败/排空actor通知，每道只一次，不覆盖已完成确认；失败无部分写入，已提交ack停服排空；交付后清理失败保留closed/唯一Delivery/图与后继资格 | 实施新增TestWorkerRuntimeCommitBarrier、TestWorkerRuntimeDeliveredCleanup与原actor/SQLite/ack入口，同步屏障枚举交错；最终基线含#95时按Plan组合运行TestDeliveryAtomicCommit；屏障/清理接线后、PR前最终候选 | 仅规划，待验收 |
| WR96-A4 | Codex原观察重接、Orca本机原终端重接/environment未知发送对账及原答案重交；三阶段输入与原会话/Attempt保持；已合入#94的未知发布merge仍正确交付，错误PR拒绝；无资源残留 | 已注册L3 controlled四场景、三个原stage integration入口，精确选择见Plan；最终基线含#94时运行TestDevelopmentUnknownPublicationMergedDelivery；独立隔离fixture，PR前及最终actor接线候选 | 仅规划，待验收 |

新包/新测试须按Plan补齐并实际枚举。原始summary/Go日志/stderr/scenario身份与清理产物保留runner新临时目录。所需子场景不允许skip/零匹配；Linux闭包不声明平台运行，controlled不替代真实Orca或Codex支持范围。固定L1仅为必要回归，不代替四项验收。

组合证据登记在本项WR96-A3/A4下，并注明最终候选基线commit及#94/#95是否已包含。未包含者记条件未触发，不记通过、不等待兄弟；已包含者必须执行对应组合，缺入口/零匹配算失败。原阶段和本项交付后清理测试始终运行，独立交付不依赖兄弟的新测试能力。同候选已执行的相同命令证据可引用，不要求重复跑。

## 人工与执行记录

无新增人工/native项；不修改安装环境。业务恢复授权仍由既有规则，未知资源/回执保留未知，不能为验收自动重派。原平台待验保持原归属。

尚未执行。按ID登记Spec/Plan固定commit、源码/dirty/构建、平台、精确命令/枚举数量、结构diff及summary/原日志/清理证据绝对目录，保留失败/修正。正式纳入后同步execution-map。本页只归属#96。
