# 仓库技能接入契约 v1

本文只说明单次 start/finish 入口。新运行应使用 `workflow_definition: minimal-v1` 和 `project_steps`，每一步单独派发并返回结构化回执；示例见 [minimal-v1 配置](../examples/config.json)。

Steerline 只配置两个技能入口。入口文件必须真实存在于目标仓库（解析 symlink 后仍在仓库内），`invocation` 必须明确描述仓库实际支持的调用方式。服务读取文件做接入检查，随后将绝对路径和调用说明传给 Codex；不会解释技能内部开发步骤。

默认提示词见 [`internal/steerline/prompt.tmpl`](../internal/steerline/prompt.tmpl)，结果 Schema 见 [`internal/steerline/result.schema.json`](../internal/steerline/result.schema.json)。两者嵌入可执行文件，每次运行保存实际 `prompt.txt` 与 Schema 副本，便于核对。

## Start

经过适配的入口包含精确标记 `<!-- steerline:start-v1 -->`，并实现以下语义：

1. 从 dispatch context 获取完整 task_id、claimant、run_id、remote、branch。
2. 核实任务已经由该 claimant 领取；复用任务，不再次 claim，不选择其他任务。
3. 核对调度器已经准备的 worktree_path、worktree_branch、base_commit，在当前 worktree 中读取 Spec 并开发。远端主线获取、隔离目录与分支创建由调度器负责；技能不再创建 worktree、切换分支或重设起点。dispatch 的 branch 是 PR 目标分支，worktree_branch 是实现分支；已有技能的默认主线与 dispatch 冲突时必须遵守显式目标，无法遵守则 blocked。
4. 使用仓库原有开发流程执行任务。

## Finish

经过适配的入口包含精确标记 `<!-- steerline:finish-v1 -->`，并实现以下语义：

1. 按原有流程完成测试、审计、修复、提交和提 PR；这些工作全部属于仓库技能。
2. PR 必须指向 dispatch 指定主线，输出明确属于当前 task_id/run_id 的 URL。
3. 提 PR 后返回等待人工合并；不合并、审批或关闭 PR，不执行 `bd close`，不改变其他任务。
4. 按 Schema 返回 `pr_submitted` 或 `blocked`，原样回传调度器提供的 worktree_path 与 base_commit，不用实现完成后的 HEAD 代替起点。没有的信息用 null。不要将会话中的自然语言总结当成结果文件。

标记表示仓库维护者已审阅和落实语义，不是对任意 Markdown 的自动安全证明。不能只加标记来绕过冲突。若入口委托其他技能，适配入口必须明确冲突处理和停止边界；遇到委托技能不可满足的要求，报告 blocked。服务不会静默改动业务仓库。

技能必须存在于被获取的目标主线提交内；配置的仓库内路径映射到新 worktree 后再次读取、检查。原 checkout 中未提交或尚未合入目标主线的技能不会自动复制过去。worker 显式继承调度器设置的 `BEADS_DIR` 和 `BEADS_ACTOR`；调度器已经从新 worktree 核对 Beads project_id、database、backend、目录及当前任务领取，技能继续复用它们，不新建或复制数据库。

本次职责调整详见 [worktree 准备与恢复](worktree-preparation.md)。这两个 v1 接入标记只用于本文描述的单次入口；当前运行快照为 schema_version=5，状态由 SQLite 权威保存。

## 合并后的 Beads 语义

默认使用本机已核对的 `bd close <id> --reason ... --actor ... --json`，再 `bd show` 验证 closed。仓库也可配置 completion 命令：服务确认合并后向它传入结构化请求，由仓库脚本调用原有生命周期、刷新并核验 BV；服务核验回执和 Beads closed 后再完成。服务不复制拆分树或自定义后继开锁逻辑。

接入仓库必须允许这种结项方式。仅作分组的 epic/molecule/convoy 和 `metadata["split.role"] == "aggregate"` 不执行。默认只允许 task/bug/feature/chore，可用 `executable_types` 缩小范围。`blocked` 任务不会因服务猜测而改成 open。

对有自定义结项事务的仓库，必须启用其 completion 适配，不能用普通 close 替代。merge-h-308-code 已新增两个专用技能及 Complete-SteerlineTask.py 适配脚本，复用现有生命周期脚本处理聚合与后继，见 [接入说明](merge-h-308-integration.md) 和 [结项命令协议](completion-command.md)。旧手动技能不被改写或在调度中调用。
