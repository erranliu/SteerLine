# 工作区与 Git 资源迁移验证

日期：2026-09-30。任务 #55；依据 [Spec](../../archive/spec/completed/execution-isolation-02-workspace.spec.md) 与 [Plan](../../archive/spec/completed/execution-isolation-02-workspace.plan.md)。状态：实现及最小验收通过，待单 PR 合入。

## 所有权与交接

`internal/steerline/workspace` 现在拥有固定本机仓库身份解析、worktree 准备、检查、精确资源目录解析及清理。Git 调用仅接收固定 executable/argv、目录和超时；进程生命周期由平台启动器提供。新包的项目内依赖只有 `execution`。

`steerline` 保留工作区意图和任务命名、领取前根目录限制、业务资格核对及回执事务。创建失败会把路径创建、Git 注册、分支存在和可检查的工作区事实写入原准备 Operation。清理未完成时写入 `native_cleanup_progress` / `outcome_unknown` 操作事实，任务清理项继续待核对；只有工作区和分支都确认不存在后，既有清理事务才结项。

`workspace.ResolveLinkedResourcesAt` 返回 objects、refs、logs 和单个 linked-worktree admin 的规范路径。`codexworker.LinkedWorktreeWriteArgs` 单独把四条路径编译成 argv；授权目录与原先一致。`info/exclude` 的本机解析也使用固定仓库身份核验。Orca terminal、dispatch 与远端 worktree I/O 仍由平台适配器执行，远端路径不会进入本机解析或清理。

## 实际验证

- `go test ./internal/steerline/workspace -count=1`：通过。真实临时仓库覆盖 prepare、inspect、资源目录、cleanup、部分准备回执、主 checkout 拒绝及其他仓库拒绝。
- `go test ./internal/steerline/codexworker -count=1 -run 'Test.*(Permission|Git|Params)'`：通过；新参数测试名称命中筛选。
- `go list -deps ./internal/steerline/workspace`：通过；项目内传递依赖仅为 `execution` 与 `workspace`。
- `go test ./internal/steerline -count=1 -run '^TestCodexLinkedWorktreeWriteArgsOnlyGrantsFixedGitPaths$'`：通过。
- `go test ./internal/steerline -count=1 -run '^TestTaskDiscardRequiresPreciseUnmergedPRAndRetainsUnknownIntent$'`：通过，包含清理部分回执保留及 cleanup pending 不结项断言。
- `go test ./internal/steerline -count=1 -run '^(TestCodexLinkedWorktreeWriteArgsOnlyGrantsFixedGitPaths|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace)$'`：通过，验证一条原 worker service 接线。未运行全量测试。
- `git diff --check`：通过。

EW1 对应依赖检查与唯一生产接线；EW2 对应有效 linked worktree、外仓身份拒绝及固定清理核验；EW3 对应四个 Git 目录与 argv 分离、准备事实和清理部分回执对账。没有运行真实 Orca、真实 Codex 权限变更或全平台故障矩阵。

## 独立复审修复

协调 session 发现 `Prepare` 在创建后只通过 `Inspect` 验证 base ancestry，丢失原实现的新工作区 HEAD 必须等于 pinned base 的检查。真实 `post-checkout` hook 追加提交可以复现错误确认。现已在确认准备成功前调用严格初始身份校验，失败仍保留注册、分支与实际 HEAD 事实；不 reset 或重建现场。`TestPrepareRejectsHeadChangedByCheckoutHook` 在修复前失败、修复后通过，workspace 包及三条受影响中心定向测试通过。
