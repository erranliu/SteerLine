# Spec：旧 Orca 编排身份残留清理

用户在当前聊天授权实施审计后的最小范围，属于普通开发，不分配 Task ID、不进入执行 DAG。实施/验证基线 `d77c399f`，提 PR 集成 `6d4b4fa2` 的文档复测成果；[Plan](../plans/retired-orca-identity-cleanup.plan.md) · [专属验收](../testing/acceptance/retired-orca-identity-cleanup.md)。

删除当前两个内置后端不再生产或消费的 Orca Run/Task/Dispatch/coordinator 身份字段，以及依赖这些字段的比较/准备条件。严格解码继续拒绝已删除字段，不迁移或读取旧开发格式。保留当前 target 的连接、runtime、host、worktree、branch/base，以及 process 的 OS 或 terminal/runtime/incarnation 身份。

删除 host 中没有调用方的 validServiceCommandKind；execution 的命令判断保持。旧 supervised 字段保留测试删除，清理回执不得改写已结算身份的测试使用当前 WorktreeID 继续验证。

不删除整套 Environment/readiness，不重写恢复、流程规则、命令映射或 UI，不新增包、接口或兼容路径。当前正常领取不创建旧准备状态；保留准备机制时只依据现存工作区/进程事实判断是否需准备。

校准架构、测试/规格/验收导航、一期准备说明、当前 execution-map 汇总及现有草稿 `SteerlineFlow/Roadmap/drafts/roadmap.md` 的能力说明，不改变任务范围、依赖、历史结算或人工安排。旧 #84/#45 的 final 缺口标注历史时点；当前能力以单终端验收为准：本机仅有限真实样本，本机最终 draft 保护未重新跑真机；远端升级 CLI 并预先确认目录信任后有限样本闭环通过，首次信任阻塞后原任务自动继续未通过，原失败保留，组合/24小时/同构建恢复待签收。不得把本次受控回归登记为新的原生验收。

完成条件为 RC-A1–A3，见专属验收。历史 Spec/Plan 和旧实验正文保留来源，不全面改写历史记录。
