# 人工决策通道（human-in-the-loop）

当 agent 无法在无人决策的情况下继续时，它不应猜、也不应发布未验证的 PR，而应**提问**。本协议让这个提问变成一个可持续、可审计、可崩溃恢复的调度状态，而不是一次性的自由文本 `blocked`。

## 原则

- **唯一权威**：只有服务写 `runtime.json`。人或查看器只写一个不可变的答案文件。
- **文件即通道**：与 `stop.json` 同构，天然崩溃安全、可脚本化、可审计、可在服务不在时回答。
- **绝不自动继续**：没有人工答案就永远停在 `waiting_decision`；重启也不自动续跑。
- **结果文件不是完成**：决策被回答并续跑后，仍要走 PR 合并验证 + Beads 关闭。

## 触发：`outcome=needs_decision`

agent 的 R-a 结果文件（`<worktree>/.steerline-result.json`）使用：

```json
{
  "task_id": "n308-21",
  "run_id": "9b57ed3f…",
  "outcome": "needs_decision",
  "pr_url": null,
  "worktree_path": "C:/…/n308-21-9b57ed3f",
  "base_commit": "8cbe82e2…",
  "summary": "…",
  "error": "…",
  "decision": {
    "question": "删除 DigHoleManager 会让老存档加载失败，怎么处理？",
    "context": "StoreData.Deserialize 遇未知 section type 会阻塞整个 Global store…",
    "options": [
      {"id": "a", "label": "保留 FullName=DigHoleManager 的纯数据兼容类型", "detail": "…"},
      {"id": "b", "label": "改共享持久化层容忍未知 section", "detail": "…"}
    ],
    "recommended": "a",
    "allow_free_text": true
  }
}
```

- `decision` 需要 `question`，且至少有 `options`（最多 12 个）或 `allow_free_text`。
- 校验规则见 `internal/steerline/decision.go`：`validateDecisionRequest`。

服务校验后置 `finish_task_status=waiting_decision`，**保留** claim / worktree / 终端，并**暂停新派发**（单执行槽留给本次决策的续跑），写出活动事件 `decision/pending`。

## 文件

| 文件 | 写者 | 说明 |
| --- | --- | --- |
| `runs/<run_id>/decision.request.json` | 服务 | 权威请求（含 `id`、`request`、`request_path`、`answer_path`），不可变 |
| `runs/<run_id>/decision.answer.json` | 人或查看器 | 原子写；服务只读 |
| `runs/<run_id>/continue.txt` | 服务 | 续跑时写给 agent 的说明（原问题 + 所选选项 + note） |

答案文件：

```json
{
  "decision_id": "14a27fb3…",
  "run_id": "9b57ed3f…",
  "option_id": "a",
  "note": "keep compat",
  "answered_by": "lmas1",
  "answered_at": "2026-09-23T03:44:01Z"
}
```

`decision_id` 必须与请求一致；`option_id` 必须是声明过的选项（无选项的决策要求非空 `note`）；`answered_by` 必填。

## 回答入口

```powershell
steerline answer -config .\service.orca.json `
  -run-id <run-id|task-id> -option <option-id> [-note "..."] [-decision-id <id>] [-by "<name>"]
```

- 只读 `runtime.json`，**不碰**任何服务状态，可在服务运行中执行。
- `-by` 缺省取 `STEERLINE_ANSWERED_BY`、`USERNAME`/`USER`，再退回 `unknown`。
- 幂等：同一 `decision_id` 重复写覆盖答案；服务只应用一次。

TTY 下 `steerline watch` 直接进入全屏查看器（`internal/steerline/viewer`）：左侧 run 列表（pending 决策置顶并标记 `DECIDE`），右侧 activity 日志，底部决策面板。按 `a` 打开弹窗，`↑↓/1-9` 选项、`tab` 切到备注、`enter` 提交、`esc` 取消；提交走与 `steerline answer` **同一个** `WriteAnswer`（只写答案文件，不改 `runtime.json`）。管道/CI 或 `--json`/`--raw`/`--no-tui` 时保持流式输出。

## 服务侧流转与续跑

1. 收到 `needs_decision` → `awaitDecision` 写 `decision.request.json`，状态 `waiting_decision`，`Decision.Status=pending`。
2. 轮询（2s）`applyDecisions`：发现合法答案 → 认领对账并核验统一执行守卫 → `Decision.Status=resuming`（先持久化）→ 事件 `decision/answered`。存在其他 unknown 执行或未结算控制副作用时保留答案等待。
3. **Tier-1 同终端续跑**：校验 `terminal show` 的 `connected/orphaned/incarnationId` 与记录一致 → 删掉旧的 worktree 结果文件 → 写 `continue.txt` → `terminal send "Read <continue.txt> and continue."` → 重新武装结果监督。
4. 成功 → `Decision.Status=applied`，`execution_status=running`，事件 `decision/applied`，Run 回到正常监督。
5. 失败 → `blocked`，事件 `decision/failed`，保留现场人工处理。

## 恢复规则（保守）

- `waiting_decision` 重启后**不自动续跑**。若已有答案文件，`applyDecisions` 会照常应用（这是人工指令，不是自动）。
- 中断在 `Decision.Status=resuming`：续跑是否发生不确定 → `unknown/blocked`，继续保留执行槽，不能直接派发替代任务。
- worktree 校验失败 → `blocked`。
- 续跑前若终端 `incarnationId` 变化或 `orphaned` → 拒绝续跑，`blocked`。

## 活动事件

`kind=decision`，`status ∈ {pending, answered, applied, failed}`；`meta`：`decision_id`、`options`、`option_id`、`answered_by`。见 `docs/activity.md`。

## 未做（后续）

- P3：Tier-2 —— 终端已死时在同一 worktree 新起 agent 续跑；决策过期策略。
