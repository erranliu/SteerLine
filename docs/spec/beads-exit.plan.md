# Plan：解除 Beads 依赖与原生接入

任务 ID：9；任务名称：解除 Beads 依赖与原生接入；目标：`BEADS-EXIT`。唯一执行依据：[Spec](beads-exit.spec.md)，共同要求见[原生任务共同约束](native-task-constraints.md)。前置[固定 DAG](task-dag.spec.md)已验收并合入。本任务由一个独立实现 PR 交付。

## 实施步骤

1. 盘点当前配置、check、worker 环境、提示词、技能、completion、viewer 和 CLI 的 Beads 耦合；明确原生路径要复用的中心 Task、Execution、Query/Command 与 PR 核验接口。
2. 使新项目只用 native 任务源和干净中心状态目录。配置冲突、旧 Beads 数据或未知状态拒绝有副作用启动；不建设导出、导入或双轨生产兼容。
3. 接入 `native-task/v1` 固定上下文及已提交的 `project_steps`；预检旧脚本对 bd/bv、start/finish 和 completion 的隐含依赖，worker 不能再次领取、关闭或解锁任务。
4. 完成本机 Codex 与已支持 Orca 的实现、验证、审查、提 PR 与合并核验闭环；受监管远端若尚未交付则保持禁用。只在真实平台事实和中心事务确认后释放后继。
5. 故障注入当前格式内的配置写入、数据库事务、派发响应、PR 查询和投影，验证原身份恢复、不重复副作用以及旧状态阻断。更新配置样例、技能与用户文档。

## 验证与交接

按 Spec NT12–NT17、NT21-EXIT 使用隔离项目完成无 Beads 的 A → B 真实闭环，并验证 local/已支持 Orca、人工决策、重启恢复和旧状态拒绝。运行针对性测试、完整 Go 测试、静态检查及 `git diff --check`；未运行的远端路径不得记为通过。向[任务关系视图](dag-view.spec.md)交接稳定的原生任务、阻塞与交付查询。
