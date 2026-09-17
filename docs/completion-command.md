# 合并后结项命令

配置为 null/省略时使用原生 `bd close`。需要仓库自定义结项流程时配置：

```json
{
  "completion": {
    "command": {
      "executable": "python.exe",
      "args": ["-B", "{worktree_path}/tools/Beads/Complete-SteerlineTask.py"]
    },
    "timeout": "2m"
  }
}
```

executable 使用原生可执行文件；PowerShell 脚本可通过 `pwsh.exe -NoProfile -File ...` 配置。参数数组中的 `{repository}`、`{worktree_path}`、`{run_dir}` 分别替换为源 checkout、本次准备的 worktree、本次运行日志目录，替换不进入 shell。默认 timeout 为 2m。命令在本次 worktree 中执行，并获得与派发一致的 BEADS_DIR/BEADS_ACTOR。

服务在派发时把 completion 配置和 Beads 身份存入运行记录。等待合并时更改全局配置不改变已有运行的处理方式，避免关闭成功但尾部失败后被错误跳过。

## 调用条件

每次调用前重新查询 PR，要求实例、仓库、URL、目标分支和真实合并证据全部匹配。再检查记录的 worktree 和 Beads 身份、任务领取者。允许任务为 in_progress 或已经 closed，后者仍必须调用适配器完成重试；不把 closed 视为回调已完成。

stdout 回执必须与本次 task_id/run_id/pr_url 一致，status=completed、error=null、schema_version=1，且命令退出 0。服务再次读取 Beads，确认该任务仍归领取者且确实 closed 后才写 finish_task_status=completed。命令失败、超时、回执无效或核验失败都保持 waiting_pr_merge，下一轮重新核实 PR 再重试，不退回普通 bd close。

## stdin 请求 v1

请求包含 schema_version、task_id、run_id、claimant、repository_path、worktree_path、worktree_branch、base_commit、remote、target_branch、hosting_url、hosting_repository、beads_context、bd，以及本轮 PR 查询返回的 pr。

- beads_context 为 `{beads_dir,project_id,database,backend}`。
- bd 为 `{executable,args}`，已解析的本机工具路径；具体仓库适配器可明确限制其支持范围。
- pr 为 `{url,repository,base_branch,state,merged,merged_at,merge_commit}`，state=merged，merged=true。
- 不传递 token、完整环境变量或模型对话。

请求先原子保存到 `runs/<run_id>/completion-request.json`，再通过 stdin 交给命令。命令不能自行选择任务、查找一个相似 PR、重新执行开发、合并 PR 或改写服务运行状态。

## stdout 回执 v1

```json
{
  "schema_version": 1,
  "task_id": "n308-14",
  "run_id": "0123456789abcdef0123456789abcdef",
  "pr_url": "http://gitea.example/owner/repo/pulls/42",
  "status": "completed",
  "error": null,
  "details": {"verification": "adapter-specific evidence"}
}
```

details 为可选的适配器证据。失败应非零退出、stderr 输出简要原因，可在 stdout 返回 status=failed、error 与部分已完成操作。服务即使命令非零退出也保存 stdout 到 completion-result.json；保留最近一轮证据，不累计完整数据库副本。

## merge-h-308-code 适配器

`tools/Beads/Complete-SteerlineTask.py` 使用 Python 3 标准库，要求 pwsh、bv 在 PATH，bd 必须是配置的原生 bd/bd.exe 且不带前置 args（既有生命周期脚本按 PATH 调用 bd）。适配器将该 bd 所在目录置于子进程 PATH 首位，固定 BEADS_DIR/BEADS_ACTOR。

它核对任务前缀、数据库身份、领取者、合并请求及既有 PR/run 绑定；调用既有 Invoke-BdTaskLifecycle.ps1 的 -DryRun，然后写入并验证 merge.pr.url 与 steerline.run_id，运行实际 Finish，再验证受影响任务、拓扑与 BV 快照。生命周期脚本继续拥有拆分树和后继状态变更逻辑，适配器只做边界核验和交接。

PR/run 绑定冲突、前缀错误或领取者不符时停止，不覆盖。任务关闭后 BV 失败，或生命周期只应用部分操作时，保留证据并重跑该幂等流程；不删除绑定、不重新领取、不回滚。适配器假定请求来自服务已验证的本轮 PR 查询，不单独持有 PR 凭据。

技能不会调用该脚本。人工 PR 合并后的轮询才会触发它。真实接入前先将技能和脚本合入配置的目标主线，使新 worktree 包含对应文件。
