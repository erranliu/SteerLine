# 仓库技能接入契约 v1

Steerline 只配置两个技能入口。入口文件必须真实存在于目标仓库（解析 symlink 后仍在仓库内），`invocation` 必须明确描述仓库实际支持的调用方式。服务读取文件做接入检查，随后将绝对路径和调用说明传给 Codex；不会解释技能内部开发步骤。

默认提示词见 [`internal/steerline/prompt.tmpl`](../internal/steerline/prompt.tmpl)，结果 Schema 见 [`internal/steerline/result.schema.json`](../internal/steerline/result.schema.json)。两者嵌入可执行文件，每次运行保存实际 `prompt.txt` 与 Schema 副本，便于核对。

## Start

经过适配的入口包含精确标记 `<!-- steerline:start-v1 -->`，并实现以下语义：

1. 从 dispatch context 获取完整 task_id、claimant、run_id、remote、branch。
2. 核实任务已经由该 claimant 领取；复用任务，不再次 claim，不选择其他任务。
3. 按仓库规则读取 Spec，获取配置的远端主线最新提交，建立隔离目录，记录真实 base_commit。已有技能的默认主线与 dispatch 冲突时，以已适配的显式参数路径处理；无法遵守则 blocked。
4. 使用仓库原有开发流程执行任务。

## Finish

经过适配的入口包含精确标记 `<!-- steerline:finish-v1 -->`，并实现以下语义：

1. 按原有流程完成测试、审计、修复、提交和提 PR；这些工作全部属于仓库技能。
2. PR 必须指向 dispatch 指定主线，输出明确属于当前 task_id/run_id 的 URL。
3. 提 PR 后返回等待人工合并；不合并、审批或关闭 PR，不执行 `bd close`，不改变其他任务。
4. 按 Schema 返回 `pr_submitted` 或 `blocked`；没有的信息用 null。不要将会话中的自然语言总结当成结果文件。

标记表示仓库维护者已审阅和落实语义，不是对任意 Markdown 的自动安全证明。不能只加标记来绕过冲突。若入口委托其他技能，适配入口必须明确冲突处理和停止边界；遇到委托技能不可满足的要求，报告 blocked。服务不会静默改动业务仓库。

## 合并后的 Beads 语义

M0 用本机已核对的 `bd close <id> --reason ... --actor ... --json`，再 `bd show` 验证 closed。依赖就绪交给原生 Beads ready，服务不复制拆分树、BV 或自定义后继开锁流程。

接入仓库必须允许这种结项方式。仅作分组的 epic/molecule/convoy 和 `metadata["split.role"] == "aggregate"` 不执行。默认只允许 task/bug/feature/chore，可用 `executable_types` 缩小范围。`blocked` 任务不会因服务猜测而改成 open。

如果仓库要求一个自定义结项事务（例如关闭子任务后递归关闭聚合、将某些后继从 blocked 改为 open），必须先调整仓库接入方式，或另行扩展并验证 Beads 适配器。M0 不宣称已经支持这类流程。对 merge-h-308-code 的具体诊断见 [接入说明](merge-h-308-integration.md)。
