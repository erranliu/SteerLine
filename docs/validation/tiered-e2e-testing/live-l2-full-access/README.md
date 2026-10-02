# L2 完全访问 smoke 验证

2026-10-02，北京时间 08:54:03–09:01:08；同一完整运行通过，无人工 sandbox 授权。

命令：`go run ./cmd/test-feedback -level=L2 -out=<新输出目录>`。

原输出：`C:/Users/lmas1/AppData/Local/Temp/sl-live-l2-full-access-fixed-20261002-085403`。
仓库内保留 [summary.json](summary.json)、各场景的清理/PR 元数据、冻结权限记录及原生 usage 证据；未复制认证、私有 SQLite 或完整原生日志。Orca 的 usage JSONL 只保留 session 身份、turn 模型/权限与原始 token_count 事件，省略原生指令正文。

## 完整结果

`coverage=full-L2`，`passed=true`，`cleanup_complete=true`，无 errors。准备约 6 秒、执行约 418 秒，总计约 7 分 4 秒。

| 场景 | 结果 | 测试耗时 | CLI 报告 tokens | 真实 PR |
|---|---|---:|---:|---|
| smoke.delivery | pass；无模型请求的受控门槛 | 65.50 s | 0 | loopback provider |
| smoke.local-codex | pass | 124.49 s | 63,927 | [#32](https://github.com/erranliu/SteerlineTest/pull/32)，MERGED |
| smoke.orca-codex | pass；真实 readiness 和业务两个回合均 succeeded | 225.16 s | 251,062 | [#33](https://github.com/erranliu/SteerlineTest/pull/33)，MERGED |

模型均为 `gpt-6-luna`，reasoning effort 为 `medium`。成功运行两条真实链路的累计 token 报告合计 **314,989**；包含 cached input，不将缓存或 before/final/cleanup 的同一累计样本再次相加，不换算账单。

## 权限和交付证据

两条链路均显式使用 `read_scope=host`、`write_scope=host`、`git_metadata_write=false`、`network=allow`、`escalation=deny`。host 写入已经包含 Git 操作，不授予额外 Git 附加根。

冻结资源契约均为 `codex-policy-v1:0.159.2:host-write:network-allow:never`，没有额外 write roots。实际 launch/turn 使用 `danger-full-access`、`approval_policy=never`，没有 Windows sandbox 设置；L2 不复制 sandbox 账号、不运行 sandbox canary。模型仍从每个项目的独立认证 home 启动。

- 本地 session：`01a0fa1c-4291-7f33-a531-1aafcb8a9559`；turn：`01a0fa1c-49be-7072-af32-1141992bff66`。PR #32 的 head 为 `71c8a9b73e5d24d3e0a7c303ed7d0959013a26bc`，merge commit 为 `174561322a752d3885f58ed421e9d21302ed8715`。
- Orca session：`01a0fa1e-5f78-7283-862f-8df166e49703`；worktree 为 `C:/Users/lmas1/orca/workspaces/repo/sl-1-47b657cd1824cefc380c798c7a9df32f`。PR #33 的 head 为 `0f045b7e1bd790de1497e504669ef3619bbccb5b`，merge commit 为 `b88e486d890477be53e3e2772f0b704944698eb1`。

两个 PR 均合并到本轮独立的 `codex/e2e/<随机ID>/<场景>/base`，并由真实 provider 确认原任务交付与后继依赖解锁。各场景清理均通过；结束后再次查询，两个 base 命名空间和两个 head ref 均为零匹配，未发现测试/helper 进程。

## 修复及定向回归

新增显式 host 写入的本地/Orca 映射；保留 workspace 模式、网络 deny 和 args 覆盖的拒绝条件。完全访问不用 Windows sandbox 实现，Orca 冻结校验不再误要求该字段。增加双链路编译/turn、无 sandbox 账号认证 seeding、冻结/恢复及配置损坏拒绝回归。

通过：

```text
go test ./internal/steerline/codexworker ./internal/steerline/execution ./cmd/test-feedback ./tests/e2e/internal/... -count=1
go test ./internal/steerline -run 'Test.*(Policy|ExecutionProfile|ManagedOrca|WindowsSandbox)' -count=1
go test ./internal/steerline -run '^(TestFullAccessProfileFreezeSurvivesClaimValidation|TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority|TestProjectPolicyMissingBoundResourcesAreNotReconstructed)$' -count=1
git diff --check
```

第一轮 `sl-live-l2-full-access-20261002-084505` 整体失败：门槛和本地 smoke 通过，本地创建/合并 [PR #31](https://github.com/erranliu/SteerlineTest/pull/31)，报告 63,515 tokens；Orca 在模型请求前因冻结校验 `stale_profile` 失败。修复后重新发起上述完整成功运行，没有把第一轮的局部成功拼成完整 L2。

本次没有重复 L3 真实调用或 full Go 回归；L3 保留原 sandbox 配置，其先前结果不作为本轮 L2 验收。sandbox 初始化脚本不属于 L2 前置条件，曾验证 UAC 触发/取消路径，授权完成后的初始化后验仍未验证。
