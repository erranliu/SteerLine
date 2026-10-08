# 状态、外部动作与恢复

## 执行来源与资格

execution-map 是用户确认的 Codex 开发任务与依赖权威。每次初始化或新派发前核对：

| 当前事实 | 处理 |
| --- | --- |
| 已确认、完整 Spec/Plan、前置满足 | 启动时固定实际文档 commit/路径，核对最新默认主干后派发 |
| 已确认、前置尚未完成 | 保留选定顺序和具体等待条件；不提前派发 |
| Spec/Plan、授权或必要环境缺失 | 记录具体阻塞；不从草稿或现场临时计划取得许可 |
| 原动作在途且清单发生变化 | 对账原聊天/worktree/请求/PR，不重复创建、不自动取消 |
| 实现完成或仅验收导航 | 核对交付与待验结果，不重新执行开发任务 |

固定需求来自实际 Git commit 内的 Spec/Plan；代码起点核对最新默认主干。初始化可包含用户选定但尚待前置的后继，逐项派发时必须重新检查；run 的串行顺序约束不能替代真实硬前置核验。run 决策中保存文档 commit/路径、依赖核验来源和结果，提示引用该版本；工作区起点不含文档时先安排可读取的文档提交进入主干，或明确从固定 commit 读取，不猜替代版本。

前置经实现、独立复审、必要验证和真实合并，验收/清单同步且成果进入最新主干后释放后继。仅余允许后置的人工项不阻塞无关任务；明确先验的人工要求、真实失败或必要能力缺失仍阻塞。清单为空或事实矛盾时不从 drafts、验收导航或旧 run 回填。

任务状态、依赖和 PR 摘要维护于 execution-map；自动/人工结果维护于任务专属 acceptance；详细聊天/worktree/SHA/审核/动作证据放仓库外 run 状态。已完成任务即使移出当前表，仍保留交付摘要和待验链接。归档通过只证明原范围/版本。

本流程不调用外部 SteerLine 中心取得开发资格或更新 TaskDelivery；具体产品测试按 Spec/Plan 在明确隔离现场运行。外部产品运行仍遵守其固定契约和业务权威。

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
  "roadmap": "C:/Project/Example/docs/roadmap/execution-map.md",
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

授权字段只记录真实授权，不照抄示意中的 `true`。roadmap绑定execution-map；tasks包含用户选定的有序任务及完整 Spec/Plan，可包含等待前置的后继；固定文档版本写入任务 decisions。助手不读取清单或核验真实依赖，无已确认任务时不初始化执行run。缺少完整Plan的旧记录不能作为执行来源。身份/顺序初始化后固定；范围变化须用户授权，原未知动作先结算，不能另建run绕过它。

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

task 的一级字段按键替换（嵌套对象须提供完整对象）。run 可更新 `phase`、`automation`、`blocker`、`decisions`、`finished_at`；身份字段不可改。用户后来明确补充或撤销授权时可更新完整 `authorization` 对象及新用户来源，助手会保留授权历史。助手仍可保存并补全待核验的 plan:null 记录，但启动正式任务前必须核对实际固定版本的 Plan，已有执行依据不可静默更换。可记录下列证据：

- `session`：thread/client ID、host、worktree、branch、base_commit、model、thinking、cursor。未解析真实 ID 时保持 `creating`；获取真实 ID 后 `running`。不能把 client ID 填进 thread ID。
- `pr`：url、number、head、base_branch、base_commit；head 更新时助手会使旧 review/validation/merge_gate 失效，并保留历史。
- `review`：head、comment_url、findings（问题/位置/影响/fix commit）、scope、base_commit；独立 review subagent 实际核验、协调 session 核对报告与证据后写入；记录审核/修复 agent ID、报告位置及对应 head/base，不能把执行者自查当作独立审核。
- `validation`：head、status（`passed`、`passed_with_manual_pending` 或已有用户明确授权的 `accepted_exception`）、commands/results；人工补验和授权例外分别记录，失败不属于可合并状态。
- `merge_gate`：head、base_commit、required_checks（`passed` 或平台已证实 `none`）、mergeable:true、checked_at。API 的 UNKNOWN 或未查询结果不等于 none；none 意味已核对该分支全部保护/规则，无必需检查。
- `merge`：head、commit、merged_at；只有平台已确认真实合并后记录。`observed_merge` 可单独记未复审就被外部合并的事实，保留待决。
- `blocker`、`decisions`：实际失败及待用户决定内容、原始用户批准记录。不得以记录例外替代取得批准。

仅余允许后置的少量人工补验时，无需逐项申请 `accepted_exception`。先实际把待验内容写入 docs/acceptance/，在 execution-map 同步验收汇总和待验链接（不改变中心状态），保留原验收条件、参与人/窗口、操作步骤及取证位置，并核对记录；后续不依赖该结果且无明确先验要求时填写：

```json
{
  "head": "reviewed-head",
  "status": "passed_with_manual_pending",
  "non_manual_status": "passed",
  "commands": ["targeted verification: passed"],
  "manual_pending": [
    {
      "check_id": "manual-check-id",
      "reason": "implemented feature awaits human interaction",
      "roadmap_path": "/absolute/project/docs/acceptance/task-acceptance.md#manual-validation"
    }
  ]
}
```

`non_manual_status` 包括实现复审、必需自动验证及其他非人工前置，必须真实通过；不能把失败、unsupported 或缺能力填成通过。`manual_pending` 是已登记人工项的定位，不是通过证据。状态助手检查元数据完整性，不代替读取清单或实际验证。PR 评论列同一清单，正常合入并推进；run 完成时保留清单及未验事实，不等待人工结果才暂停 heartbeat。用户要求先完成人工验收、或人工验收已出现真实失败时，继续使用原阻塞规则。

`roadmap_path` 是现有状态格式中的验收记录定位字段，值指向 acceptance 文档；协调者实际核对记录并同步 execution-map 的待验链接。元数据不代替文档明细或正式汇总。

已合入 task 的原审核、验证和合并证据仍不可修改；补验结果另写 docs/acceptance/ 的具体项并同步正式汇总，可在 run.decisions 保留定位。原 `manual_pending` 保留的是合入时尚未人工验证的事实；这些外部流程记录不改变中心 Task/DAG 状态。人工为主活动不作为执行成员，明确先验门槛及已知失败仍阻塞。

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

## 工作分流与独立 subagent

协调 session 尽量保持调度与证据核对职责，不默认亲自解决冲突或补完原任务。判断依据是实际工作内容，不以 diff 大小或出现在哪个文件决定归属：

- **交回原执行 session**：PR/rebase/merge 冲突、最新主干或共同接口集成、Spec 已要求但实现/原测试断言/实际消费者遗漏、验收未接通及需要重新组织原实现。用原 thread、worktree 和 PR，描述具体缺口/冲突与证据、当前 main/head 和必要验证；不另开替代执行 session，不只发“继续”。若原需求/Spec 本身缺失或矛盾，先让原 session 核对原请求、整理范围内修订建议；需要扩大范围或更改验收时仍由用户决定，不能让执行者自行豁免。
- **独立 review subagent**：每个候选新建只读 reviewer，给固定 Spec/Plan、任务/PR/worktree、精确 head/base、约束和真实证据入口，不复制执行 session 的实现过程或预设“应该通过”。先独立查完整 diff、生产调用路径及最小必要验证；报告问题/位置/影响、可复核证据、验证状态和未解决项。协调者核对结果并分流，不以 reviewer 一句“通过”直接合并。
- **独立 fix subagent**：确认的局部缺陷且无需补做原需求时，新建另一个 repair agent，给问题证据、允许修改范围、候选身份与必要验证，在同一 PR 分支修复并提交，报告真实 commit/命令/结果后停止写入。冲突、实质需求遗漏等工作仍优先原 session；原 session 无法继续时先记录原因及交接再选择修复者，不自动创建替代执行聊天。

review/fix 用 `collaboration.spawn_agent` 的 subagent，不用 `create_thread` 制造新的用户执行聊天。它们默认沿用协调 session 模型；仅执行 session 受 Luna/xhigh 固定设置约束，用户另有模型指令时遵守。用最小必要上下文建立独立判断，reviewer 与 fixer 分开；修复或执行者续跑后用新的只读 reviewer 检查最终改动，必要时复用未受影响的真实验证，避免整套重复运行。delegation 不可用时记录具体限制，不能把协调自查或原执行自查伪称为独立 subagent 复审。

每次交接前，协调先在任务 `decisions` 保存候选 head/base、工作分类、目标执行 thread/agent、允许写入范围和意图；subagent 创建返回身份后立即用状态助手补记 agent ID/报告位置。发送原 session 提示仍用 `reserve send_prompt`、固定模型/effort、成功 `apply/settle`；未知发送结果先对账。review/fix agent 不写共享 run 状态，也不自行发送 PR 评论/合并或执行 Git push；修复 commit 由协调核对后统一推送、发表评论和合并。

同一 worktree 只允许一个写入者：原执行 session、fix subagent 与协调不能重叠写入；只读 reviewer也只在候选静止时检查。交回执行者前结束现有 review/fix 工作并收齐报告，记录新写入所有者；修复者停写后才交给终审 reviewer。跨 heartbeat 保存 agent ID、状态与报告，先核对现有工作再恢复；暂时看不到、超时或报告未返回不能重复创建同一轮 agent。

head 或 base 变化后旧审核/门禁失效；冲突解决或需求补做完成的执行者仍须自查、验证和推送原 PR，然后停止写入。协调重新核对真实候选，调度独立审核，保存最终报告及必要验证后才进入原合并门禁。已经合入任务的历史审核方式与模型证据保持原样，新流程只作用于后续工作。

## 合并证据与恢复

`ready_to_merge` 要求 pr.head、review.head、validation.head、merge_gate.head 相同，review/merge_gate 的 base 与当前记录的 PR base 相同，评论 URL 存在，验证满足上述通过/人工登记/已授权例外条件，平台检查通过。`merged` 额外要求 merge.head 相同、merge commit 及 merged_at 非空。head 或 base 改变会清掉旧证据，退回 pr_ready；不能拿旧审核直接合并。实际合并操作前仍须再次读取平台，JSON 校验不消除竞争窗口。

恢复每个任务时核对：session 是否仍写入，worktree 的 HEAD/分支/dirty 状态，PR 最新 head/base/状态/检查；记录与现实矛盾时保留旧证据并记录原因。未经复审的外部合并不自动过关；关闭 PR 不自动另开。暂停/完成时都通过工具更新原 heartbeat，不新增替代 automation。最终任务合并后先将 run 标记 complete，再暂停 automation；暂停响应未知时保留 pending，下次只核对/补完成停用，不再次执行任务。

每个 run 只有登记的协调 session 负责状态写入、外部动作调度与合并。原执行 session 处理实现、冲突和需求遗漏；协调 session 明确委派的 fix subagent 可以在限定 worktree/范围修复，不能自行接管协调身份、改 run 状态、发送其他聊天消息或合并 PR。只读 review subagent 不修改文件。短期文件锁/CAS 保护状态事务，不保护整段代码审核或 Git 操作，不能靠它让两个协调 session 同时推进。heartbeat 的活跃 turn 与新用户指令按同一协调聊天串行处理。

## 离线验证助手

维护本技能后运行 `python <skill>/scripts/test_state.py -v` 和 skill-creator 的 `quick_validate.py <skill>`。这些测试仅用临时目录验证重复唤醒、未知结果恢复、串行门禁、固定执行模型、审核失效及完成停用；不创建真实 session、automation 或 PR，不能据此宣称实际外部平台流程已经验证。
