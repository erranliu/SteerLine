# Rebase 到 PR #185

2026-10-02：本分支基于包含 PR #185 的 `origin/main` 合并提交 `84097f3d0856d7b5d16036a53f6ed82016fb3593`。该提交是当前 HEAD 的祖先。

初次 rebase 基线为 `7e79a9b`（#183/#184）；用户补充要求后再次 fetch/rebase 到 #185。本地原始备份为 `codex/backup-tiered-e2e-2d1c98c`，远端更新使用精确旧提交的 force-with-lease。

## 适配

- 保留 #63 的 identity-bound implementation envelope 与 #185 的 lifecycle ports、原始输入恢复及原子领取逻辑。
- E2E 配置改为 version 2 implementation_configs。模型/effort、app-server session、Orca turns 和 policy resource 通过当前实现 codec 按项目/执行/attempt/generation/config 身份校验后读取；不读旧字段或新增兼容路径。
- 完全访问契约接入新 policy resource codec，Orca host 写入策略继续可选；无 bounded write roots 的断言保持。
- 选择和冻结共用冻结 envelope 的摘要，避免相同配置因两种投影算法产生 stale_profile。#185 原子领取的 freeze 校验保留。
- 修复合并后的 fixture：新的配置/发布权威、codec 绑定参数，以及“已有 process 且缺少原始资源”的拒绝场景，不制造真实执行证据。

## 验证

通过：

```text
go test ./cmd/test-feedback ./tests/e2e/internal/... ./internal/steerline/codexworker ./internal/steerline/orcaworker -count=1
go test -tags=e2e ./tests/e2e/live ./tests/e2e/smoke ./tests/e2e/suites/... -run '^$' -count=1
go run ./cmd/test-feedback -level=L2 -env=controlled -out=<新目录>
git diff --check
git merge-base --is-ancestor 84097f3d0856d7b5d16036a53f6ed82016fb3593 HEAD
```

#185 上的主包 `Test.*(Policy|ExecutionProfile|ManagedOrca|WindowsSandbox|Lifecycle)` 定向运行只有 `TestProjectPolicyMissingBoundResourcesAreNotReconstructed` 的旧 fixture 失败。补齐该用例要求的 process 证据后，单独重跑它、完全访问冻结与实际领取回归，全部通过；其余定向用例此前已通过，没有重复整个运行。

最终无 token smoke 输出：`C:/Users/lmas1/AppData/Local/Temp/sl-rebase185-l2-controlled-20261002-120402`；summary 为 passed=true、cleanup_complete=true。初次基线的 L2/L3 controlled 检查也通过，但不代替 #185 上的结果。

本次未调用真实模型、未新建 GitHub 测试 PR、未执行 full。此前 L2/L3 真实模型通过记录属于 rebase 前历史证据，未声称在新基线上重跑过。
