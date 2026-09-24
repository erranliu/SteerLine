# 调度器准备 worktree

这是用户确认的 M0 职责调整：调度器准备 Git 工作环境，技能负责在该环境里开发、验证、提交和提 PR。实现不包含资源缓存准备、依赖安装、Unity 启动或仓库开发流程。并发执行上限仍为 1，获取主线和准备 worktree 也占执行槽。

## 配置与命名

`worktree_root` 默认 `<run_dir>/worktrees`，显式相对路径相对配置文件目录。它必须在所有已有 checkout 和 Git 元数据之外，且与运行记录目录不重叠；同目录的父级 junction/symlink 会先解析。不会编辑业务仓库的 ignore 配置。

每次运行：

- 目录：`<worktree_root>/<run_id>`。
- 分支：`codex/<task_id>-<run_id前12位>`。
- 获取基线的本地引用：`refs/steerline/bases/<run_id>`。

任务 ID 必须可以安全组成分支名。创建前检查已有路径与分支，使用独占目录创建防止冲突，不使用 force/reset 或已有 worktree。准备失败留下的目录、分支和引用保留现场。

## 派发顺序

1. 持有仓库与运行目录锁，核对历史领取，查询 Beads ready。
2. 为候选分配 run_id，标记执行槽用于准备；只读获取当前共享 Beads 的目录、project_id、database 和 backend。
3. `git fetch --no-tags --no-recurse-submodules --no-write-fetch-head -- <remote> refs/heads/<branch>:refs/steerline/bases/<run_id>`，解析并固定该引用的完整 commit hash。不通过可能被其他进程修改的 FETCH_HEAD 选择起点。
4. 将任务 ID、运行 ID 和 workspace_intent 原子持久化，再原子领取并核实任务；领取冲突不创建 worktree。
5. 领取后再次持久化；`git worktree add --no-track -b <implementation-branch> -- <path> <base_commit>`。仅原 checkout 的 Git 元数据发生必要变化，不切换原 checkout 的分支或改动它的工作文件。
6. 核验新目录、Git common-dir、分支和 HEAD；将配置技能的仓库相对路径映射到该目录，重新检查入口文件及接入契约。
7. 设置 `BEADS_DIR` 为源环境已核实的共享目录、`BEADS_ACTOR` 为 claimant。在新 worktree 用同样环境读取 `bd context --json` 和任务，要求数据库身份、任务 ID、领取者和 in_progress 一致。数据库内部文件不被复制或修改。
8. 把核验后的 worktree_path、worktree_branch、base_commit 持久化，然后以 worktree 为 cwd 启动 Codex，传入既有结构化结果 Schema。

新的工作环境使用选定主线提交中的技能和文件。未提交的本地技能、仅在其他分支的技能不会自动复制；缺少必要入口时 blocked。认证继承现有 Git/Codex 环境；Git 非交互读取超时或失败不触发静默授权提升。

## 失败、恢复与保留

fetch 或源 Beads context 失败时没有任务被领取，后续轮询可重试。领取后，任何 worktree/技能/共享 Beads 检查失败都保留领取，记录 failed/blocked，不启动 Codex、不自动重试该任务。

创建意图早于创建行为落盘：准备途中崩溃后可能只有意图，也可能已经有目录和分支。恢复会只读核对现场，将遗留 pending/running 标记为 unknown/blocked，核对进程身份，不自动重建目录或重启执行。`mark-stopped` 可在人工核实后释放保留的执行槽；它不会自动重跑任务或补造准备事实。

正常执行结束后、人工 accept-result 时及恢复未完成记录时，重新核对工作环境。实现 HEAD 可前进，但应保持原 base_commit 在历史中，并留在分配的实现分支；结果回传的路径和起点必须与调度器记录一致。目录丢失、分支被改动或起点历史不匹配会阻塞，保留原记录。

等待 PR 时保留 worktree，释放执行槽供其他独立任务使用。完成后本阶段也不自动删除目录、分支或基线引用；清理应停机后人工确认无未完成任务、无未保存改动，再使用 Git worktree 管理命令处理。不要把整个 worktree_root 当作普通日志目录批量删除。

运行观察投影写 schema_version=5。v1–v4 快照不兼容，读取器拒绝且保持原文件不变；执行状态从 `run_dir/steerline.db` 恢复。具体字段见 [数据契约](data-contract.md)。

共享 Beads 的定位使用其公开环境变量接口：[Beads worktree 共享数据库说明](https://github.com/gastownhall/beads/discussions/1233)。启动仍以本机 `bd context --json` 的实际返回值和两端身份比较为准。
