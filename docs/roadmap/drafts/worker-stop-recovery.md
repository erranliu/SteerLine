# Worker 停机对账与恢复一致性修复

本轮选定草稿，依据 2026-10-09 用户对 101 的系统性修复、简化和 worker 生命周期边界要求。沿用 #87；只规划，未实现、未纳入 execution-map，未修改安装数据或启动任务。

| 任务 ID | 任务名称 | 成果 / 所有者 | 前置与共享影响 | 规格 | 专属验收 |
| --- | --- | --- | --- | --- | --- |
| 87 | Worker 停机对账与恢复一致性修复 | worker 层停服行为标识/规范恢复事实；execution 应用统一资格与提交 | 无未交付业务硬前置；与 #84 共用 worker 恢复契约，有序集成 | [Spec](../../spec/worker-stop-recovery.spec.md) · [Plan](../../spec/worker-stop-recovery.plan.md) | [验收](../../acceptance/worker-stop-recovery.md) |

Worker 声明并冻结 `ServiceStopBehavior`：local Codex 为 stop_execution，停服释放精确 owned 执行且保留现场；Orca 为 continue_execution，只脱离观察、原执行可继续。不尝试优雅生成 final。标识是预期，实际 running/question/completed/interrupted/unknown 由 worker 核实返回；应用恢复只与该层打交道，不直接处理 PID/endpoint/thread 或 terminal/Dispatch。

复用现有生命周期端口、规范事实、ClassifyRecovery、单写提交和 Decision/Command 幂等。新增最小冻结标识，升级受影响版本、拒绝旧格式，不迁移或默认填值；旧安装 101 仅是诊断证据，不承诺跨构建接管。支持格式现存停止意图由 worker 自己核实，业务未知仍阻塞。没有通用操作框架、独立恢复权威或新 suite。

同一任务、一个 PR；WSR-A1–A8 分别覆盖标识与架构边界、两种动作/实际事实、答案与结果/隔离、双受控产品集成、local 真实停止恢复及 Orca 活动停服/离线完成重接。复用现有组和 driver，最小能力缺口在 Plan 明确补齐；无独立人工任务。

范围待用户确认纳入后固定实际 Spec/Plan commit/路径并核对主干。建议 #87 先交付共同恢复契约供 #84 消费；如 #84 在途，先核对原规格和实际改动再确认有序集成，不静默中断或改写。此草稿不改正式 #84→#45→#44 排期，不创建外部 Task/DAG。安装处置、一期组合及 24 小时签收另行安排。
