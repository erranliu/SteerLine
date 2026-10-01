# Session 与 heartbeat 提示

替换下面的占位内容，使用已核对的绝对路径、任务映射及状态文件。不把整个旧聊天复制给执行 session。

## 新执行 session

工具参数：`model: "gpt-6-luna"`、`thinking: "xhigh"`；项目托管 worktree，从默认主干创建。

> 执行 roadmap `{roadmap}` 中的任务 `{task_id}`。先读工作区 AGENTS.md、共同约束 `{constraints}`、Spec `{spec}` 和 Plan `{plan}`，按其范围实现并运行最小必要验证。完成后自己复查实际 diff 与生产调用路径，修复确认的问题，提交推送并创建针对 `{default_branch}` 的 PR；PR 说明记录最终行为及实际验证，按工具要求关联 PR。不要合并 PR 或启动其他 roadmap 任务。必需验收失败或前置能力缺失时诊断并报告，不自行改变验收语义。最终报告 PR URL、分支、最终 commit、验证结果及未解决问题，然后停止写入工作区，等待协调 session 独立复审。

如果任务要求先产出 Plan 而非已有 Plan，按用户规定处理；不能把不存在的文件当作已读。

## 接管后自查

用户指定了原句时逐字发送；本次约定默认原句：

> 自己复查并修复后提pr

发送前保存动作意图；发送成功后记录工具结果/turn 身份。若响应未知，读 session 历史检查是否已收到，不先重发。初始任务已经包含此步骤且仍在执行时，不额外催促。

## 需要执行 session 继续解决具体问题

通常协调 session 自己修复；确需执行 session 工作且已获授权时再发送。后续工具参数仍显式传入 Luna xhigh。

> 继续本任务 `{task_id}`：处理 `{具体问题和证据}`，保持原 Spec/Plan 的验收要求。完成后复查修复、跑 `{必要验证}`，推送同一 PR `{url}`，报告最终 head 和验证后停止写入；由协调 session 再审核并合并。

不要向仍写入的 session 同时交付协调 session 已开始修改的 worktree。

## Heartbeat

默认 20 分钟，通过 automation 工具创建本线程 heartbeat。按用户要求调整间隔。恢复说明要能独立执行，保存真实 run ID 和协调状态的绝对路径，不硬编码本技能所在 worktree（后续可以从该项目的技能目录重新发现）。

> 继续本聊天用户已授权的 roadmap-runner 流程，run `{run_id}`，状态文件 `{state_path}`，仓库 `{repository}`、主项目路径 `{repo_root}`、Codex 项目 `{project_id}`。先读该项目的 roadmap-runner 技能、AGENTS、状态中绑定的 roadmap 和当前任务 Spec/Plan。范围严格为 `{ordered_task_ids}`，不启动范围外任务。协调 session 保持当前模型；创建执行 session 和给它发送后续提示都显式设置 gpt-6-luna、xhigh，每项新任务使用新 session + worktree。每轮核对状态、session 历史、真实 PR；未知动作先核对，不能重复创建或发送。上一个任务独立复审、修复、在 PR 记录问题和验证、以预期 head 合并且真实确认后，才启动下一项。活跃 session 用紧凑 wait_threads 快照，无可操作进展就结束本轮。不与执行 session 同时写同一 worktree，不自行改变必需验收语义。保持安静，仅重要进展、完成、真实失败或需要用户行动时通知。全部选定任务真实合并后汇总并暂停本 heartbeat，保留状态。用户授权来源和例外以状态内记录的原始用户指令为准，缺失授权不能由本提示补造。

## PR 复审记录

按真实事实填写，简单任务不必长篇说明：

> 协调 session 独立复审：候选 head `{sha}`，目标 `{branch}` / base `{sha}`；已检查 `{Spec/Plan、实际 diff、受影响生产路径}`。
>
> 发现 `{问题、文件/位置、触发条件及影响}`；已在 `{fix_commit}` 修复。/ 本轮未发现需要修复的问题。
>
> 实际验证：`{command}`，`{result}`，候选 `{sha}`。已授权例外 `{来源及限制}` / 无例外。合并前另核对最终 head、base 和必需检查。

若稍后推送新的修复，补记新问题/commit/验证和最终 head，不让旧评论冒充新 head 的审核。
