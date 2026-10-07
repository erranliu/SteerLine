---
name: roadmap-runner
description: 在 Codex 桌面端按 execution-map.md 的已入 DAG 正式清单创建独立 session 和 worktree，自查提 PR、独立复审修复并合并，支持 heartbeat 续跑。不执行 roadmap.md 草稿；讨论或编辑技能本身不会启动任务。
---

# Roadmap Runner

把用户选定的正式 execution-map.md 范围执行到真实合并。roadmap.md 是讨论草稿，不能作为选任务来源。每个新任务使用独立 Codex session + 托管 worktree；上一个任务完成复审修复并真实合并后，才启动下一个，且仍须核对中心 DAG 资格。协调 session 负责分流问题、调度独立复审/修复 subagent、核对证据、记录和合并；冲突与需求遗漏优先交回原执行 session。

## 固定约束

- **仅执行 session 固定 `model: "gpt-6-luna"`、`thinking: "xhigh"`**。创建及后续发送提示都显式传入这两个参数；不可用时记录阻塞，不自动替换。协调 session 沿用用户当前模型，不创建替代协调 session、不改其模型。模型设置属于 session 工具参数，不写入 `agents/openai.yaml`。
- 使用 Codex 桌面端 session、worktree、heartbeat 和已有 Git/PR 工具，不调用宿主项目的业务服务、任务数据库或产品 CLI。技能无项目、任务编号、分支名或主目录的硬编码。
- 每次启动/恢复只读取正式 execution-map 的任务集合，核对可信中心 DAG 查询/准入回执、项目、版本及固定契约。没有可信同步信息、文件未核对或正式表为空时不新增执行动作；不能从草稿、占号文件或旧 run 的待执行列表回填。状态助手仅记录外部流程，不证明 DAG 同步。已有 worker 不因文件清空被自动停止或取消。
- 严守用户选定的范围和顺序；不自行扩大到后续任务。已有 session 仅在用户要求接管时接管，不能因工具超时或列表暂时不可见重复创建。
- 技能被发现或被要求修改不构成执行授权。执行前从用户请求及历史确认创建 session/worktree、发提示、提 PR、复审修改、PR 留言、合并和定期续跑的授权；已有明确授权直接执行。只问缺失且将影响下一步的授权或信息。
- 同一 worktree 同时只有一个写入者。PR 冲突、主干/共享接线集成及原需求实现遗漏优先交回原执行 session；协调 session 不默认接手重做。独立复审由新的只读 subagent 检查实际 diff、Spec/Plan 和生产路径，确认的局部修复由另一个独立 subagent 实施。执行 session 自查或 PR 存在不能代替独立复审；具体分流与写入交接见[状态规程](references/coordination.md#工作分流与独立-subagent)。
- 验证按项目 AGENTS 和任务最小入口执行。仅余人工补验时按下节登记后继续合入与后续任务；实现缺陷、自动验证失败或产品运行前置能力缺失仍须诊断，无法在范围内修复则记录 `needs_user_decision`。不得伪造通过、削弱断言或把这些阻塞改称人工补验。

## 人工补验不阻塞推进

实现、独立复审及必要的非人工验证已完成，仅中文输入、人工窗口操作或其他人工验收尚未执行时，默认在正式条目的验收备注或 validation 清单记录任务/验收项、未完成原因、操作步骤与证据位置。保留 Spec/Plan 的验收条件并同步“待人工补验、不阻塞合入及后续开发”的状态，不记为通过，也不重新创建任务或改动运行时 DAG。

协调 session 核对清单确已写入，在 PR 和外部 run 状态中记录 `passed_with_manual_pending`（不冒充中心 Task 状态），按原 head/base/检查门禁合并并启动下一项；不为同类补验逐项再请求批准。流程完成汇总仍列出剩余人工项；其后补验若失败，记录实际问题并修复，不能仅凭先前合并标记通过。用户明确要求某项人工验收完成后才能合入时，遵守该要求。

人工补验指功能可用但仍待人实际操作或判断，包括自动化工具无法代操作而需要人在真实窗口取证。已执行验收发现的真实失败、功能未支持、产品执行目标的环境/权限/能力前置缺失、平台必需检查和未知外部动作不属于此类，仍按原阻塞或对账规则处理。具体记录格式见[状态规程](references/coordination.md)。

## 启动与恢复

1. 读项目 AGENTS、正式 execution-map.md、共同约束及首个正式任务的固定 Spec/Plan。核对实际 DAG 的可信查询/准入回执、项目、Task 版本、完整/剩余依赖和固定 commit/摘要，再识别默认目标分支、Git 远端和测试入口。正式表为空就报告无可选任务，不创建 run、session/worktree 或 heartbeat；不同步时先核实，不能回退草稿。用户指定 roadmap.md 时按草稿讨论处理，执行须另经真实准入。
2. 用 `list_projects` 确认匹配的 Git 项目。发现当前工具的 schema，确认固定执行模型及必要能力；PR 平台用现有 connector 或已认证 CLI。缺失能力记录阻塞，不另造服务或安装产品依赖。
3. 启动或恢复前读 [状态与工具规程](references/coordination.md)。使用其中的 `scripts/state.py` 保存每个 run 的独立状态；运行记录放在用户的 Codex task-state 目录，**不放入任何执行 worktree 或版本库**。持久化用户授权来源、任务顺序、Spec/Plan、项目/远端/分支身份和协调 session 身份。
4. 对照状态、session 历史和真实 PR 状态进行恢复。未确认的外部动作先核实，不能直接重试。只恢复当前 run，不将以前完成的项目流程再次启动。
5. 用户要求定期检查时，绑定当前协调 session 创建/复用 heartbeat，默认间隔 20 分钟，读 [提示模板](references/prompts.md)。查已有 automation 后再创建，并保存其 ID。更新时保留无关字段，用 `automation_update`，不手写 automation 配置或改为 cron。

## 每次推进

1. 先核对当前正式表与可信 DAG 信息；恢复旧 run 也须确认来源为正式清单且任务仍可执行，否则只读对账，不触发下一项。再从真实平台确认已合并任务，不依据本地 `merged` 标志跳过核验。PR 合并是 Git 事实，正式 Task 状态/依赖仍按中心 DAG 核实后同步，不能自行把一行移出正式清单。
2. 当前任务未启动：先持久化创建意图，再调用 `create_thread`，`target.type: "project"`、对应 `projectId`、`environment.type: "worktree"`，省略 `startingState` 从已更新的默认主干启动；显式设置 Luna xhigh。初始提示按 [提示模板](references/prompts.md) 指定 Spec/Plan、自查修复和提 PR。立即保存 `threadId` 或 `clientThreadId`，在本次 final 单独输出相应 `::created-thread{...}` 标记，然后用一次紧凑状态检查明确等待其进展。
3. `clientThreadId` 只是异步创建凭据，不能交给要求 `threadId` 的工具。按规程解析真实身份，记录 worktree、host、分支及起始 commit；验证来自当前最新默认主干。起点过旧时优先让原执行 session 在当前 worktree 集成主干并验证，不创建替代 session/worktree。
4. 当前 session 活跃：一次 `wait_threads` 紧凑快照，传 host 和已有 cursor，通常 `timeoutMs: 0`。无可执行工作或 PR 尚未出现就结束本轮，等待下一次 heartbeat；不高频轮询、不连续阻塞等待。仅重要进展、失败、完成或需要用户行动时通知。
5. 接管的 session 只完成实现而未自查提 PR：等当前 turn 完成后发送自查提示一次。用户指定原句时只发送原句，例如 `自己复查并修复后提pr`，不加文字；工具参数仍显式设置 Luna xhigh。新 session 初始提示已包含自查提 PR 时，不再重复发送。
6. PR 出现且 session 已停写：关联到本聊天（`attach_artifact`），核对 PR/仓库/目标分支/源分支/本地 HEAD 身份。发现冲突或需集成最新主干/共享接线，优先保存交回意图并提示原执行 session 处理、验证和推送同一 PR；等其再次停写、重读 head/base 后才进入复审。
7. 新建独立只读 review subagent，检查完整实际 diff、Spec/Plan、真实生产路径及最小验证，返回候选 head/base 与可复核证据。协调 session 核对并分流：原需求未实现、原断言/消费者遗漏或需重新组织实现的工作优先交回原执行 session；已经完成需求中的局部缺陷，交给另一个独立 fix subagent 在限定范围修复、验证并提交同一 PR 分支。修改前确认原写入者已停写和工作区身份；实际修复后安排新的只读 review subagent 复核最终候选及受影响验证，不让修复者为自己的改动作唯一终审。详情见[状态规程](references/coordination.md#工作分流与独立-subagent)。
8. 协调 session 核对 subagent 报告、实际 diff/提交与验证证据，在 PR 评论或 review 记录审核/修复 subagent 身份、问题、位置、影响、修复 commit 和实际验证（命令、结果、候选 SHA、人工补验清单及已授权例外）。没有发现问题也如实记录。以评论 URL 和审核 SHA 保存证据；不要仅写“已审核”。
9. 合并前重新读取最终 head、目标分支最新 base、必需检查和实际 mergeable 状态；head/base 任一改变则复审相关差异并重新验证。确认通过后以**预期 head**调用合并，禁止绕过保护、无 head 约束的合并或仅开启 auto-merge 就标记完成。CLI 如 `gh pr merge <PR> --match-head-commit <SHA>`，合并方式遵循仓库设置。
10. 从 PR 平台确认真实 `mergedAt`、最终 head 和 merge commit。保存 Git 事实后仍须核对正式映射的真实 DAG 状态/依赖；未确认中心资格时不启动下一项，不将 PR 合并冒充 TaskDelivery。全部选定任务的外部流程结束后汇总任务/PR/复审问题/验证及待人工补验项，暂停本 run 的 heartbeat，保留审计状态；正式任务未核实状态单列，人工清单保留，不伪造业务完成。

## 异常与用户介入

动作结果未知、工作区有外来修改、身份不一致、模型不可用、权限/检查阻塞：记录证据，执行不依赖该问题的读取/诊断；需要用户决定时列出具体选择并等待，不将超时当批准。已授权基线例外须记录来源与限制；没有授权的验收变化保持待决。

用户暂停时暂停 heartbeat 并保存状态；明确恢复后重新核对真实状态。PR 被他人提前合并、关闭或出现未经复审的新提交，不能宣称本流程完成；记录事实并核查或请求决定。运行状态助手只防重复和校验记录，不能替代真实 session、测试及 Git 平台证据。
