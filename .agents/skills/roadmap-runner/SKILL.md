---
name: roadmap-runner
description: 在 Codex 桌面端按 roadmap 顺序创建独立 session 和 worktree，执行任务、自查提 PR，由协调 session 独立复审修复并合并，支持 heartbeat 续跑。用于请求执行或恢复这套流程；讨论或编辑技能本身不会启动任务。
---

# Roadmap Runner

把用户选定的 roadmap 范围执行到真实合并。每个新任务使用独立 Codex session + 托管 worktree；上一个任务完成复审修复并真实合并后，才启动下一个。协调 session 负责复审、修复、记录和合并。

## 固定约束

- **仅执行 session 固定 `model: "gpt-6-luna"`、`thinking: "xhigh"`**。创建及后续发送提示都显式传入这两个参数；不可用时记录阻塞，不自动替换。协调 session 沿用用户当前模型，不创建替代协调 session、不改其模型。模型设置属于 session 工具参数，不写入 `agents/openai.yaml`。
- 使用 Codex 桌面端 session、worktree、heartbeat 和已有 Git/PR 工具，不调用宿主项目的业务服务、任务数据库或产品 CLI。技能无项目、任务编号、分支名或主目录的硬编码。
- 严守用户选定的范围和顺序；不自行扩大到后续任务。已有 session 仅在用户要求接管时接管，不能因工具超时或列表暂时不可见重复创建。
- 技能被发现或被要求修改不构成执行授权。执行前从用户请求及历史确认创建 session/worktree、发提示、提 PR、复审修改、PR 留言、合并和定期续跑的授权；已有明确授权直接执行。只问缺失且将影响下一步的授权或信息。
- 不在执行 session 仍写入时修改同一 worktree。协调 session 独立阅读实际 diff、Spec/Plan 和受影响生产路径；执行 session 自称完成、测试通过或 PR 已存在都不能代替复审。
- 验证按项目 AGENTS 和任务最小入口执行。必需验收失败或前置能力与 Spec 冲突时，先诊断；无法在范围内修复则记录 `needs_user_decision`。不得自行替换验收语义、削弱断言或豁免必需检查以取得通过。

## 启动与恢复

1. 读项目 AGENTS、roadmap、共同约束及首个待执行任务的 Spec/Plan。识别明确任务列表、依赖、默认目标分支、Git 远端和测试入口。范围或映射有歧义时先澄清，不猜测任务。
2. 用 `list_projects` 确认匹配的 Git 项目。发现当前工具的 schema，确认固定执行模型及必要能力；PR 平台用现有 connector 或已认证 CLI。缺失能力记录阻塞，不另造服务或安装产品依赖。
3. 启动或恢复前读 [状态与工具规程](references/coordination.md)。使用其中的 `scripts/state.py` 保存每个 run 的独立状态；运行记录放在用户的 Codex task-state 目录，**不放入任何执行 worktree 或版本库**。持久化用户授权来源、任务顺序、Spec/Plan、项目/远端/分支身份和协调 session 身份。
4. 对照状态、session 历史和真实 PR 状态进行恢复。未确认的外部动作先核实，不能直接重试。只恢复当前 run，不将以前完成的项目流程再次启动。
5. 用户要求定期检查时，绑定当前协调 session 创建/复用 heartbeat，默认间隔 20 分钟，读 [提示模板](references/prompts.md)。查已有 automation 后再创建，并保存其 ID。更新时保留无关字段，用 `automation_update`，不手写 automation 配置或改为 cron。

## 每次推进

1. 从真实平台确认已合并任务。找第一个未合并任务，不依据本地 `merged` 标志跳过核验。
2. 当前任务未启动：先持久化创建意图，再调用 `create_thread`，`target.type: "project"`、对应 `projectId`、`environment.type: "worktree"`，省略 `startingState` 从已更新的默认主干启动；显式设置 Luna xhigh。初始提示按 [提示模板](references/prompts.md) 指定 Spec/Plan、自查修复和提 PR。立即保存 `threadId` 或 `clientThreadId`，在本次 final 单独输出相应 `::created-thread{...}` 标记，然后用一次紧凑状态检查明确等待其进展。
3. `clientThreadId` 只是异步创建凭据，不能交给要求 `threadId` 的工具。按规程解析真实身份，记录 worktree、host、分支及起始 commit；验证来自当前最新默认主干。起点过旧时在后续审核前集成主干并检查，不启动并发任务来补救。
4. 当前 session 活跃：一次 `wait_threads` 紧凑快照，传 host 和已有 cursor，通常 `timeoutMs: 0`。无可执行工作或 PR 尚未出现就结束本轮，等待下一次 heartbeat；不高频轮询、不连续阻塞等待。仅重要进展、失败、完成或需要用户行动时通知。
5. 接管的 session 只完成实现而未自查提 PR：等当前 turn 完成后发送自查提示一次。用户指定原句时只发送原句，例如 `自己复查并修复后提pr`，不加文字；工具参数仍显式设置 Luna xhigh。新 session 初始提示已包含自查提 PR 时，不再重复发送。
6. PR 出现且 session 已停写：关联到本聊天（`attach_artifact`），核对 PR/仓库/目标分支/源分支/本地 HEAD 身份，集成已合入主干及共享接线，进入独立复审。
7. 协调 session 阅读完整实际 diff、Spec/Plan、受影响调用路径和必要测试；亲自修复确认的问题，跑最小必要验证，提交并推送到同一 PR 分支。先确认工作区干净，保留用户已有修改。提交或主干集成后复审受影响的最终内容。发现额外新提交时使旧审核失效。
8. 在 PR 评论或 review 记录问题、位置、影响、修复 commit 和实际验证（命令、结果、候选 SHA、已授权例外）。没有发现问题也如实记录。以评论 URL 和审核 SHA 保存证据；不要仅写“已审核”。
9. 合并前重新读取最终 head、目标分支最新 base、必需检查和实际 mergeable 状态；head/base 任一改变则复审相关差异并重新验证。确认通过后以**预期 head**调用合并，禁止绕过保护、无 head 约束的合并或仅开启 auto-merge 就标记完成。CLI 如 `gh pr merge <PR> --match-head-commit <SHA>`，合并方式遵循仓库设置。
10. 从 PR 平台确认真实 `mergedAt`、最终 head 和 merge commit，保存后才能启动下一项。全部选定任务合并后汇总任务/PR/复审问题/验证，暂停本 run 的 heartbeat，保留审计状态。

## 异常与用户介入

动作结果未知、工作区有外来修改、身份不一致、模型不可用、权限/检查阻塞：记录证据，执行不依赖该问题的读取/诊断；需要用户决定时列出具体选择并等待，不将超时当批准。已授权基线例外须记录来源与限制；没有授权的验收变化保持待决。

用户暂停时暂停 heartbeat 并保存状态；明确恢复后重新核对真实状态。PR 被他人提前合并、关闭或出现未经复审的新提交，不能宣称本流程完成；记录事实并核查或请求决定。运行状态助手只防重复和校验记录，不能替代真实 session、测试及 Git 平台证据。
