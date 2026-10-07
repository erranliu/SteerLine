# Plan：解除 Beads 依赖与原生接入

任务 ID：9；任务名称：解除 Beads 依赖与原生接入；目标：`BEADS-EXIT`。状态：实现与验收完成，等待实现 PR 审阅。唯一执行依据：[Spec](beads-exit.spec.md)，共同要求见[原生任务共同约束](../../../spec/native-task-constraints.md)。前置[固定 DAG](task-dag.spec.md)已验收并合入。本任务由一个独立实现 PR 交付。

## 实施步骤

- [x] 盘点配置、check、worker 环境、提示词、技能、completion、viewer 和 CLI 的 Beads 耦合；原生路径复用中心 Task、Execution、Query/Command 与 PR 核验接口。
- [x] 使新项目只用 native 任务源和干净中心状态目录。配置冲突、旧 Beads 数据或未知状态拒绝有副作用启动；没有导出、导入或双轨生产兼容。
- [x] 接入 `native-task/v1` 固定上下文及已提交的 `project_steps`；预检旧脚本对 bd/bv、start/finish 和 completion 的隐含依赖，worker 不再领取、关闭或解锁任务。
- [x] 完成本机 Codex 与已支持 Orca 的实现和验证；受监管远端未交付，保持禁用。只在真实平台事实和中心事务确认后释放后继。原生执行端在副作用前核对固定工作区/base 与契约，启动回执丢失后按原身份对账。
- [x] 接入已丢弃任务的实际清理：以固定身份停止已验证的本机 worker，对不支持的远端取消保留人工待办；开放 PR 和工作区分别清理并记录回执，未知结果继续占用。复用中心迟到合并冲突事实，不补写正常交付。
- [x] 故障注入当前格式内的数据库事务、派发响应、清理回执和投影，验证原身份恢复、不重复副作用以及旧状态阻断。native 派发模式和暂停状态只保存在中心 SQLite，没有配置文件镜像写入面。更新配置样例、技能与用户文档。

## 验证与交接

按 Spec NT12–NT17、NT21-EXIT、NT22-EXIT 使用隔离项目完成无 Beads 的 A → B 真实闭环，并验证 local/已支持 Orca、人工决策、重启恢复、丢弃清理和旧状态拒绝。针对性测试、`go test ./... -count=1 -timeout 20m`、`go vet ./...` 和 `git diff --check` 均通过；未运行的远端路径没有记为通过。向[任务关系视图](dag-view.spec.md)交接稳定的原生任务、阻塞与交付查询。真实平台和包级测试证据见[验收记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/beads-exit/README.md)。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../validation/README.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
