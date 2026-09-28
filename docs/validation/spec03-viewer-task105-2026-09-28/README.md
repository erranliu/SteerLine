# 台式机64g：SteerlineTest Task 105 的可见步骤转换

2026-09-28，使用已运行的 SteerlineTest 项目服务和台式机64g 受监管远端。仅创建新的原生 Task 105；先前已通过的 home 故障矩阵、当前主机 Decision 2.215 秒样本和 Tasks 101–104 均未重跑。[固定任务输入](task-batch.json)引用测试仓库 `54021d66a9174cec9fb8e56bed6cb084c703a1d3` 的独立 Spec/Plan。原 WorkflowRun `a31472a8802f72f95234fd6897d1e719`，执行 `4e8d38f2e5f0a5dc440638a58f2af51a`，选定配置为 Orca 台式机64g Codex Luna xhigh。

## 画面测量

首次提前监听 prepare→implement 时，控制台仍在“Ready”筛选：Task 从列表移出并丢失详情选中对象。[该次记录](first-step-miss.json)在 12 秒内没有同 Attempt 的可见画面，**不计为 4 秒通过样本**。随后切到稳定的“全部”筛选，固定 Task 105 详情，确认 implement Attempt 已显示，再提前启动下一次监听。

[有效的 implement→verify 记录](step-visible.json)从中心 SQLite `step_attempts` 的新 verify Attempt `9626bc5212ecebf8afc59ac371d53481` 的持久 `created_at=2026-09-28T09:42:35.9727643Z`，量到 Orca 可见终端 `source=screen` 首次显示**同一** `attempt_id`、Task 105、WorkflowRun 与执行身份的画面。读取结束时间为 `09:42:37.306449Z`，包括 SQLite 轮询、Orca CLI 和画面读取的保守上界 **1.334 秒**，通过用户为当前主机设定的 **4 秒** 门槛。没有把内部 View 渲染时间或先前迟到画面当作端到端时延。

## 原 Task 结算

原 Run 的 prepare、implement、verify、review、publish_pr 五步均 succeeded。固定候选 `5f4b19653e8dd948ff03a2f3bd6313610861a19d` 的 [SteerlineTest PR #23](https://github.com/erranliu/SteerlineTest/pull/23) 仅新增 `evidence/native-105.md`；核对目标 `main`、唯一文件 Diff 和精确 head 后人工合并，提供方 merge commit `8b07c225da96670fe7d85f0c2218668310cadfdb`。服务窗口曾在合并后、中心 PR 对账前以 `context canceled` 停止；同一 SQLite 和配置重开后，中心查询真实合并并将原 Task 105 记为 `closed`，WorkflowRun `completed`，五笔资源释放 Operation `confirmed`，原五个 Orca Dispatch 均 `completed/exited`。[中心与平台核对](closure.json)保留这些独立回执。服务保持 `running/ready`。

该计时证明一次**完整交付 Run**中的真实远端步骤转换可见性；它本身不证明 #6 的全部受影响故障路径，也不自动交付原中心 #6 Task。首次筛选丢失对象和服务窗口中断均保留为独立试次事实。
