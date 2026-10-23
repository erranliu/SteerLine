# Orca 工作区创建超时调整验收

归属：2026-10-14 当前聊天直接授权的工作区创建超时小改，不属于 roadmap 调度任务。

## 范围

仅 Orca `worktree create` 调用使用现有项目 `CommandTimeout`；没有配置值时沿用 40 秒默认值。终端查询预算、固定基线、候选、恢复、worker 和 PR 流程保持现有行为。

## 自动验收

| ID | 验收项 | 结果与证据 |
| --- | --- | --- |
| OCT-01 | 本机及 paired Orca 创建调用使用配置的 2 分钟预算 | 已通过；`TestWorkspaceCreationUsesConfiguredTimeout` 的本机与 `paired-worker` fixture 均确认创建成功且仅调用一次 |
| OCT-02 | 后续终端查询保持原有预算及工作区恢复约束 | 已通过；上述新场景核对查询为 40 秒，已有工作区终端准备及回执恢复场景通过 |

执行命令：`go test ./internal/execution/adapters/orcatransport -run 'TestWorkspace(Creation|Terminal)' -count=1`。

结果：exit 0，目标包 `ok`，耗时 2.777 秒。场景使用现有隔离 fixture，无模型请求。`git diff --check` 和 `build.ps1` 通过。没有运行全量或真实 PR 测试。

## 安装与后续验证

- 安装位置：`C:/SteerlineRuntime/bin/steerline.exe`。
- 已核对安装文件与本次构建 SHA-256 相等：`E7BB8C7B1E1583B5CAEF365B825FB2B2BD406258A6A1A1B24400A95A6232942D`。
- 原程序保存在 `C:/SteerlineRuntime/backup/orca-workspace-timeout-20261014-102124/`。安装配置和任务数据库没有直接改写。
- 安装版 `check -config C:/SteerlineRuntime/etc/merge-h-308-code.json` exit 0，配置、native authority、Git 和 PR provider 检查通过，未领取任务。
- 正式停服请求 `7261f4b8df85dfcd8af4d4f36b17918f` 返回 applied，并确认原进程退出和锁释放；用户重开原 Windows 服务窗口后加载新程序。
- 真实远端缺少基线提交时的创建结果尚待验证；本记录不宣称 40 秒超时是全部失败原因，也不将原失败候选自动重派。
