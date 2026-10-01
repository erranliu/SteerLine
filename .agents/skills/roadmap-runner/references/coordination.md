# 状态、外部动作与恢复

## 身份及状态存放

每次运行绑定一个协调 session、一个 Git 仓库和一份明确有序任务列表。run ID 在该用户的 task-state 目录内唯一（例如 `roadmap-<repo>-<时间或 UUID>`），不要只用任务编号作为 ID。

默认状态目录：`$CODEX_HOME/task-state/roadmap-runner`；未设置时用 `~/.codex/task-state/roadmap-runner`。选定后持久化绝对路径。多个 worktree 共享同一份 run 状态，不各自复制。工具返回内容和测试摘要足够，勿存凭据或整个日志。

助手 `scripts/state.py` 仅操作 JSON 文件，使用标准库，不创建 session、不调用网络、不修改仓库。命令的 `--expect` 是上次读取的 revision；不匹配即拒绝，重新读状态再核对，不绕过。写入使用独占锁及同目录临时文件原子替换。锁不会自动过期；异常遗留的 `.lock` 只有确认没有在运行的状态写入者后才可删除，不能按等待时长猜测。

## 初始化

先把已核对的信息写入 UTF-8 JSON 输入文件。最小输入示意（路径须替换成实际绝对路径）：

```json
{
  "run_id": "unique-run-id",
  "repository": "owner/repo",
  "repo_root": "C:/Project/Example",
  "project_id": "verified-project-id",
  "coordinator_thread_id": "current-thread-id",
  "roadmap": "C:/Project/Example/docs/roadmap/roadmap.md",
  "default_branch": "main",
  "authorization": {
    "source": "本聊天用户指令或可核对的原始消息引用",
    "create_sessions": true,
    "send_prompts": true,
    "publish_prs": true,
    "review_fix": true,
    "comment_prs": true,
    "merge_prs": true,
    "schedule": true
  },
  "interval_minutes": 20,
  "tasks": [
    {"id": "#A", "spec": "C:/Project/Example/docs/a.spec.md", "plan": "C:/Project/Example/docs/a.plan.md"},
    {"id": "#B", "spec": "C:/Project/Example/docs/b.spec.md", "plan": "C:/Project/Example/docs/b.plan.md"}
  ]
}
```

授权字段只记录真实授权，不照抄示意中的 `true`。不存在的 Plan 可设为 `null`，同时在执行提示中明确按用户流程产出 Plan。当前 run 的任务身份/顺序初始化后固定；修改范围需明确用户授权并另建 run，不扩写原运行。

```text
python <skill>/scripts/state.py init --state <absolute-state-path> --input <input-json>
python <skill>/scripts/state.py show --state <absolute-state-path>
python <skill>/scripts/state.py apply --state <absolute-state-path> --expect <revision> --input <patch-json>
```

`apply` 输入只支持以下两个键，省略不用的键：

```json
{
  "run": {"phase": "active", "automation": {"id": "verified-id", "status": "ACTIVE"}},
  "tasks": {
    "#A": {
      "phase": "creating",
      "session": {"client_thread_id": "returned-client-id", "model": "gpt-6-luna", "thinking": "xhigh"}
    }
  }
}
```

task 的一级字段按键替换（嵌套对象须提供完整对象）。run 可更新 `phase`、`automation`、`blocker`、`decisions`、`finished_at`；身份字段不可改。用户后来明确补充或撤销授权时可更新完整 `authorization` 对象及新用户来源，助手会保留授权历史。原先 `plan: null` 的任务允许绑定一次后来按原范围产出的 Plan 路径，已有映射不可换。可记录下列证据：

- `session`：thread/client ID、host、worktree、branch、base_commit、model、thinking、cursor。未解析真实 ID 时保持 `creating`；获取真实 ID 后 `running`。不能把 client ID 填进 thread ID。
- `pr`：url、number、head、base_branch、base_commit；head 更新时助手会使旧 review/validation/merge_gate 失效，并保留历史。
- `review`：head、comment_url、findings（问题/位置/影响/fix commit）、scope、base_commit；只有协调 session 真实独立审核后写入。
- `validation`：head、status（`passed` 或已有用户明确授权的 `accepted_exception`）、commands/results、exceptions 的用户授权来源；失败不属于可合并状态。
- `merge_gate`：head、base_commit、required_checks（`passed` 或平台已证实 `none`）、mergeable:true、checked_at。API 的 UNKNOWN 或未查询结果不等于 none；none 意味已核对该分支全部保护/规则，无必需检查。
- `merge`：head、commit、merged_at；只有平台已确认真实合并后记录。`observed_merge` 可单独记未复审就被外部合并的事实，保留待决。
- `blocker`、`decisions`：实际失败及待用户决定内容、原始用户批准记录。不得以记录例外替代取得批准。

task phase：`pending → creating → running → pr_ready → reviewing → ready_to_merge → merged`；停顿用 `blocked` 或 `needs_user_decision`。run phase：`active / paused / needs_user_decision / complete`。每次读取/写入都校验基本证据；助手不自动推断 Git 或 session 状态。

## 外部动作先留意图

创建 session、发送提示、PR 评论、合并、创建/暂停 heartbeat 之前，先保留动作键及精确请求摘要（提示文本可保存，凭据不保存）。可重复的动作以 head/用途区分键；例如 `#A:self-review`、`#A:merge:<sha>`。同键不得重复预留。每个 run 同时只允许一个 `pending` 执行动作；暂停 heartbeat 可以另行预留一次，避免未知创建/合并结果阻止用户停用调度。这种状态意味着“未确认”，不意味着“未发生”。

```text
python <skill>/scripts/state.py reserve --state <path> --expect <revision> --key <action-key> --kind <kind> --task <task-id> --input <request-json>
python <skill>/scripts/state.py settle --state <path> --expect <revision> --key <action-key> --status confirmed --input <evidence-json>
```

kind 可为 `create_session / send_prompt / comment_pr / merge_pr / create_heartbeat / pause_heartbeat`。run 动作省略 `--task`。创建/发送的 request 摘要必须有 `model` 和 `thinking`；合并摘要必须有 `expected_head`；评论摘要必须有 `head`，另保存实际参数/提示。创建预留会立即把任务标为 creating。`reserve` 成功后才调用外部工具；CAS/锁冲突不得调用。将返回的 client/thread/PR/automation 身份立即 `apply` 保存，再 `settle confirmed` 记录返回证据。出错或崩溃后的 pending 不清理，不按超时重试；先查工具真实状态。

只有外部已证实**未产生副作用**（如参数校验明确拒绝且没有创建）才 `settle --status not_applied`，保存核对证据后可以使用新的 attempt 键重试。请求超时/连接断开/列表未找到不足以证明未发生。无法核实则保持 pending、报告阻塞。这些机制提供保守的防重复，不声称跨平台“恰好一次”。

合并时先 reserve，调用具备预期 head 的平台合并，再平台查询确认 merged，最后 apply merged 证据、settle。请求响应未知但平台已显示合并的已审核 head，可确认而不重试。PR 评论通过 run/action 标记及实际评论查询核对，避免响应丢失重复发表评论。

## 异步 session 创建

记录 `create_thread` 完整身份结果，在 final 单独显示：

```text
::created-thread{threadId="真实 threadId"}
```

或异步返回：

```text
::created-thread{clientThreadId="真实 clientThreadId"}
```

用 `list_threads` 获取真实身份，再 `wait_threads` 紧凑确认，不用 client ID 冒充 thread ID。仅创建成功但列表暂时没出现时，可在同一主机只读 `$CODEX_HOME/sessions` 对应时间范围的 rollout `session_meta`，以工具已知 cwd、标题/任务、创建时间验证身份，再由 thread 工具确认。读取应限于候选 metadata，不批量转储其他聊天或敏感日志。多个候选或没有证据就等待/报告，不猜 ID、不重复创建。

接管已有并行任务时在初始化输入中逐个记录 `adopted: true` 和真实 session/PR/模型历史。允许它们先完成当前 turn，但仍按选定顺序复审合并。历史 turn 的模型不能伪报为 Luna；下一次提示显式选择 Luna xhigh，确认后保存执行模型。新建任务不能借 adopted 绕过串行规则。

## 合并证据与恢复

`ready_to_merge` 要求 pr.head、review.head、validation.head、merge_gate.head 相同，review/merge_gate 的 base 与当前记录的 PR base 相同，评论 URL 存在，验证和检查通过。`merged` 额外要求 merge.head 相同、merge commit 及 merged_at 非空。head 或 base 改变会清掉旧证据，退回 pr_ready；不能拿旧审核直接合并。实际合并操作前仍须再次读取平台，JSON 校验不消除竞争窗口。

恢复每个任务时核对：session 是否仍写入，worktree 的 HEAD/分支/dirty 状态，PR 最新 head/base/状态/检查；记录与现实矛盾时保留旧证据并记录原因。未经复审的外部合并不自动过关；关闭 PR 不自动另开。暂停/完成时都通过工具更新原 heartbeat，不新增替代 automation。最终任务合并后先将 run 标记 complete，再暂停 automation；暂停响应未知时保留 pending，下次只核对/补完成停用，不再次执行任务。

每个 run 只有记录的协调 session 可以推进外部动作和修复文件；其他聊天只读检查或在用户明确交接后接管。短期文件锁/CAS 保护状态事务，不保护整段代码审核或 Git 操作，不能靠它让两个协调 session 同时推进。heartbeat 的活跃 turn 与新用户指令按同一协调聊天串行处理。

## 离线验证助手

维护本技能后运行 `python <skill>/scripts/test_state.py -v` 和 skill-creator 的 `quick_validate.py <skill>`。这些测试仅用临时目录验证重复唤醒、未知结果恢复、串行门禁、固定执行模型、审核失效及完成停用；不创建真实 session、automation 或 PR，不能据此宣称实际外部平台流程已经验证。
