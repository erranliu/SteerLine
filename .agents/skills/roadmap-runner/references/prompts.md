# Session 与 heartbeat 提示

替换下面的占位内容，使用已核对的绝对路径、任务映射及状态文件。不把整个旧聊天复制给执行 session。

roadmap只绑定用户确认的execution-map。协调者先按[来源与资格](coordination.md#执行来源与资格)核对状态、依赖和固定Spec/Plan；等待前置者不派发，旧run不能扩大当前范围。讨论或修改本技能不触发这些执行模板。

## 新执行 session

工具参数：`model: "gpt-6-luna"`、`thinking: "xhigh"`；项目托管 worktree，从默认主干创建。

> 执行正式 execution-map `{roadmap}` 中已核对前置的任务 `{task_id}`。先读工作区 AGENTS.md、共同约束 `{constraints}`、Spec `{spec}` 和 Plan `{plan}`（固定文档 commit `{document_commit}`，按该 commit 读取），按其范围实现，逐项完成Plan映射的自动验收和最小必要回归，在当前任务专属acceptance登记真实结果。完成后自己复查实际 diff 与生产调用路径，修复确认的问题，提交推送并创建针对 `{default_branch}` 的 PR；PR 说明记录最终行为及实际验证，按工具要求关联 PR。不要合并 PR 或启动其他 roadmap 任务。仅余人工补验时在 SteerlineFlow/testing/acceptance/ 登记任务/验收项、原因、步骤和证据位置，保留未验事实，在execution-map同步当前任务的验收汇总/待人工链接，未完整排入的未来节点不加入索引；仅在后续不依赖该少量人工结果且无明确先验要求时后置，不仅因此保持 draft 或请求批准。真实验收失败、功能或前置能力缺失仍诊断并报告，不改称人工补验。最终报告 PR URL、分支、最终 commit、验证结果、人工清单及未解决问题，然后停止写入工作区，等待协调 session 调度独立 subagent 复审；后续冲突、共享主干集成或原需求遗漏优先由本执行 session 处理。

发送前必须核对已有固定Plan。若用户要先规划或补Plan，按规划请求处理，不用本执行模板启动尚不具备执行条件的worker。

## 接管后自查

用户指定了原句时逐字发送；本次约定默认原句：

> 自己复查并修复后提pr

发送前保存动作意图；发送成功后记录工具结果/turn 身份。若响应未知，读 session 历史检查是否已收到，不先重发。初始任务已经包含此步骤且仍在执行时，不额外催促。

## 需要执行 session 继续解决具体问题

PR 冲突、主干/共享接线集成及原需求遗漏优先交回原执行 session；原流程已有发送授权时直接发送一次，后续参数仍显式传入 Luna xhigh。发送前结束已委派的 review/fix 工作并交接唯一写入者，不新建替代聊天。

> 继续本任务 `{task_id}`、原 worktree `{worktree}` 和 PR `{url}`：以最新 `{default_branch}` / `{base_sha}` 为基线，处理 `{冲突文件/原需求遗漏及具体证据}`，保持原 Spec/Plan 的验收要求。完成后复查修复、跑 `{必要验证}`，推送同一 PR `{url}`，报告最终 head 和验证后停止写入；由协调 session 调度新的独立 subagent 复审并按门禁合并。

不要在原 session、repair agent 或协调者仍写入时交给另一位写入者。

## 独立审核与修复 subagent

reviewer 使用新 subagent、只读，并沿用协调模型：

> 独立复审任务 `{task_id}` 的 PR `{url}`，固定候选 head `{sha}`、base `{base_sha}`、worktree `{worktree}`。先读 AGENTS、唯一 Spec/Plan 和约束，检查完整实际 diff、真实生产调用路径与最小必要验证。不要修改文件、run 状态、评论 PR、合并或提示其他聊天。不要依据执行者“通过”结论作判断；报告候选身份、问题/位置/影响与可复核证据、实际验证结果、未解决项和报告位置。区分局部缺陷与原需求遗漏/集成工作，供协调分流。

fixer 使用另一个新 subagent；先确认当前唯一写入者已停写：

> 修复任务 `{task_id}` 在独立审核中确认的 `{问题/证据}`，限定 worktree `{worktree}`、PR `{url}`、head `{sha}`、base `{base_sha}` 和修改范围 `{scope}`。保持固定 Spec/Plan 和原断言；跑 `{最小必要验证}`，在原分支提交并报告实际 commit/命令/结果，然后停止写入。不要自行 push、评论/合并 PR、改 run 状态或提示其他聊天。出现冲突、实质需求遗漏或需扩大验收时报告给协调，优先交回原执行 session。你的报告不是最终独立审核，由新的只读 reviewer 复核改动。

## Heartbeat

默认 20 分钟，通过 automation 工具创建本线程 heartbeat。按用户要求调整间隔。恢复说明要能独立执行，保存真实 run ID 和协调状态的绝对路径，不硬编码本技能所在 worktree（后续可以从该项目的技能目录重新发现）。

> 继续本聊天用户已授权的 roadmap-runner 流程，run `{run_id}`，状态文件 `{state_path}`，仓库 `{repository}`、主项目路径 `{repo_root}`、Codex 项目 `{project_id}`。先读该项目的 roadmap-runner 技能、AGENTS、状态中绑定的 roadmap 和当前任务 Spec/Plan。范围严格为 `{ordered_task_ids}`，不启动范围外任务。协调 session 保持当前模型；创建执行 session 和给它发送后续提示都显式设置 gpt-6-luna、xhigh，每项新任务使用新 session + worktree。每轮先核对execution-map范围、状态、依赖和明确前置，再核run、session历史和真实PR；退出范围、前置未满足或事实未知时只对账原动作，不新派发。未知动作先核对，不能重复创建或发送。冲突、主干集成和原需求遗漏优先交回原执行 session；协调新建独立只读 review subagent，局部修复交给另一独立 fix subagent，修复后再独立复核。上一个任务独立复审、修复、在 PR 记录 agent 身份/报告、问题和验证、以预期head合并且真实确认，同步acceptance/execution-map且前置成果进入最新默认主干后，才启动下一项。活跃 session 用紧凑 wait_threads 快照，无可操作进展就结束本轮。不与执行 session 同时写同一 worktree。仅余允许后置的少量人工补验时登记当前任务acceptance明细并同步execution-map待人工链接，不加入尚未完整排入的未来阶段；自动补验不改称人工，归档通过不当成本次通过，记录 passed_with_manual_pending 后正常合入推进，不重复申请单项批准；真实失败或缺环境/权限/能力仍保留阻塞，不伪造验收通过。保持安静，仅重要进展、完成、真实失败或需要用户行动时通知。全部选定任务完成实现、独立复审、必要验证、真实合并及acceptance/execution-map同步后，汇总PR/实现交付与待人工项，将run标记完成并暂停本heartbeat；保留交付记录与待验链接，不等待允许后置的人工结果。用户明确的先验收再合入要求、授权及其他例外以状态内原始指令为准。

## PR 复审记录

按真实事实填写，简单任务不必长篇说明：

> 协调 session 已核对独立 review subagent `{review_agent_id}` 的报告 `{report}`：候选 head `{sha}`，目标 `{branch}` / base `{sha}`；已检查 `{Spec/Plan、实际 diff、受影响生产路径}`。
>
> 发现 `{问题、文件/位置、触发条件及影响}`；由原执行 session 或 fix subagent `{fix_agent_id}` 在 `{fix_commit}` 修复；最终 review subagent `{final_review_agent_id}` 已复核。/ 本轮未发现需要修复的问题。
>
> 实际验证：`{command}`，`{result}`，候选 `{sha}`。待人工补验 `{验收项及roadmap定位}` / 无人工残留。已授权例外 `{来源及限制}` / 无例外。人工登记不表示通过；其余验证完成后正常合入推进。合并前另核对最终 head、base 和必需检查。

若稍后推送新的修复，补记新问题/commit/验证和最终 head，不让旧评论冒充新 head 的审核。
