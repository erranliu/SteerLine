# Orca 工作区启动时序修复

## 范围

修复工作区创建后终端表面归属尚未就绪导致启动中断的问题。
沿用现有 lifecycle operation/receipt，不新增表、配置或通用重试机制。
创建结果未知且没有原始回执的历史执行不自动接管、不迁移。
验收归属本次普通修复，见 [专属验收](../testing/acceptance/orca-workspace-startup.md)。

## 实施

1. 在工作区准备步骤内，terminal list/show 共用现有 wait_timeout 截止时间；仅重试
   terminal_surface_ownership_unavailable，空列表继续等待，其他错误直接返回。
2. 成功创建的 runtime、工作区实例、base/branch/path，以及已查询到的原终端身份，
   写入现有 operation receipt；超时、查询失败也保留。立即 reconciliation 接收同一回执。
3. 恢复仅从保存回执查询原工作区和终端，核对身份后完成绑定，再执行现有 agent-start。
   没有回执、多个终端或身份改变时停止，不重建、不重发启动命令。

## 测试计划

全部自动项及登记位置见 [专属验收](../testing/acceptance/orca-workspace-startup.md)。

| 验收项 | 级别/场景 | 环境与命令 | 时点与通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| OWS-A1 | L1 / TestWorkspaceTerminalSurfaceReadiness | 隔离 fake；`go test ./internal/execution/adapters/orcatransport -run '^TestWorkspaceTerminalSurfaceReadiness$' -count=1 -v` | 修改后；暂时不可用能成功，共用期限且创建仅一次 | 定向 Go 日志；验收 OWS-A1 |
| OWS-A2 | L1 / TestWorkspaceCreationReceiptRecovery、TestLifecycleEffectForwardsWorkspaceCreationCheckpoint | 隔离 fake；`go test ./internal/execution/adapters/orcatransport -run '^TestWorkspaceCreationReceiptRecovery$' -count=1 -v`；`go test ./internal/execution -run '^TestLifecycleEffectForwardsWorkspaceCreationCheckpoint$' -count=1` | 修改后；序列化后重建 adapter 可恢复，只读核验，反复失败不丢失/嵌套回执 | 定向 Go 日志；验收 OWS-A2 |
| OWS-A3 | L1 / TestWorkspaceCreationRecoveryRejectsChangedIdentity | 隔离 fake；`go test ./internal/execution/adapters/orcatransport -run '^TestWorkspaceCreationRecoveryRejectsChangedIdentity$' -count=1 -v` | 修改后；无回执、身份变化或多终端时拒绝，零资源写操作 | 定向 Go 日志；验收 OWS-A3 |
| OWS-A4 | 独立 live / TestOrcaWorkspaceStartupLive | 先 `scripts/new-debug-release.ps1 -Orca` 生成本机 Orca 及 agent_launch 包；设置 `STEERLINE_ORCA_WORKSPACE_PROBE_ROOT=<包根>`；`go test ./internal/execution/adapters/orcatransport -run '^TestOrcaWorkspaceStartupLive$' -count=1 -v -timeout=4m` | 定向回归后；正常启动和注入查询失败后原回执恢复，真实 main-agent 就绪，各只创建/启动一次，清理成功 | 包 evidence/workspace-startup-live.log 与 orca-startup-*.json；验收 OWS-A4 |

必要回归：完整 Orca adapter/protocol 两个小包、任务领取未决恢复相关两条测试及
`go test ./internal/testkit/boundary/... -count=1`。不运行 full、L2/L3 交付场景：本次不改变
任务实现、发布、PR 或交付逻辑；真实探针只请求 READY，不执行业务任务。

人工窗口：无新增人工验收门槛；安装环境 101 的旧执行处置不在本次范围。
