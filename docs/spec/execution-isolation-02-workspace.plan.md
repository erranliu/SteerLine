# Plan 工作区与 Git 资源迁移

任务 ID：55；任务名称：工作区与 Git 资源迁移；目标：`EXECUTION-ISOLATION`。状态：已实施，待合入。唯一执行依据：[Spec](execution-isolation-02-workspace.spec.md)。硬前置：#54 已合入，一个实现 PR 交付。

## 迁移步骤

1. 接收 #54 的仓库、工作区和操作契约，列出 `workspace.go`、Git sandbox 与准备/清理消费者，区分中心意图和 Git I/O。
   对照 contracts 的命令/业务记录与 execution 的工作区/目标值，保留单一定义；远端路径不进入本机解析/清理，不重复记录或查询层。
2. 创建 workspace 包，先搬仓库身份/路径解析和 inspect，再搬 prepare/cleanup。保留领取前根目录校验与中心纯命名意图，不搬入 Task 丢弃资格。通过小输入提供 Git command、目录、预算和必要环境；不引入通用 Runner 框架或父包 Config。
3. 把 linked worktree 的真实资源解析放入 `resources.go`，Codex argv 映射迁入 `codexworker/permissions.go`，更新本机和现有 Orca 调用。保持原可写目录与参数语义。
4. 中心依然先保存意图再调用操作，回执不明确时保留原资源；迁移已有临时 Git fixture，删掉旧实现和重复 helper。
5. 与 #56 的集成人核对共享文件，后合入方 rebase 并检查发布调用仍使用正确仓库和分支。

## 验证

```powershell
go test ./internal/steerline/workspace -count=1
go test ./internal/steerline/codexworker -count=1 -run 'Test.*(Permission|Git|Params)'
go list -deps ./internal/steerline/workspace
go test ./internal/steerline -count=1 -run '^(TestCodexLinkedWorktreeWriteArgsOnlyGrantsFixedGitPaths|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace)$'
```

资源精确性测试搬迁后使用新包对应名称，确认实际执行；新包只包含本轮迁入的必要 Git/清理场景。依赖与生产调用审查对应 EW1；临时 Git 身份及重定向场景对应 EW2；资源参数拆分和一个部分准备/清理接线对应 EW3。不要同时复制旧测试矩阵，也不跑全量或真实 Orca。

## 交付

记录工作区操作、中心意图/回执写入点、Codex 权限 helper 及留给 #58 的 Orca I/O。#57 在本任务和 #56 均合入后实施完整 Codex 适配迁移。
