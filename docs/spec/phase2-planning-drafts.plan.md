# Plan：规划提案与受控草稿

任务 ID：17；任务名称：规划提案与受控草稿；目标：`PHASE2`。唯一执行依据：[Spec](phase2-planning-drafts.spec.md)。一个独立实现 PR 交付。

硬前置：#16。全部按原任务交付核验，不能按接口里程碑提前执行。

## 实施顺序

1. 以 #16 当前接口建立规划领域表和 DTO；实现 RegisterPlanningWorkspace、人工 fast-forward 同步、固定 Git 文档基线及离线读模型。
2. 先实现共用 guard 与持久 owner/generation，使用模拟 turn/publish 消费者验证争抢、旧 token 与恢复阻塞，后继直接调用同一入口。
3. 扩展共享 Git ref 分配器为按 request 摘要的原子批次预留，接 SQLite Operation；验证与现有 CLI 同时占号及各个响应丢失窗口。
4. 实现提案生命周期、受管正文版本与草稿投影，接路径/配额和既有 DAG/任务结构校验。
5. 实现正式/草稿/GoalRef/差异读模型、needs_review 重审和失效诊断，按 PD1–PD9 在临时真实 Git worktree 与 SQLite 验证后交接给会话/发布消费者。

## 验证与交接

按本任务验收编号逐项记录于 docs/validation/phase2-planning-drafts/；保存数据库/文件/Git/外部身份和前后状态，不以最终文本代替应用事实。先跑领域定向和故障测试，再跑受影响 Go 全量回归、go vet 与 git diff --check，服从所引用共同 Spec 的测试门槛。需要真机的项单列环境和未测项。公共接口变化同时更新消费者 Spec、schema 和契约样例，不通过添加旧协议兼容掩盖不一致。
