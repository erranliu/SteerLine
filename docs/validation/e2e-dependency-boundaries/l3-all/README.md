# 全部 L3 验收

2026-10-02（Asia/Shanghai），六个登记场景均通过，所有现场清理完成。controlled 与 live 分开，四个真实场景各自独立执行，Windows 原生资源串行。真实模型固定 gpt-6-luna / medium，full access；L3 使用 loopback Git/PR provider，没有创建外部 GitHub 测试 PR。

| 场景 | 环境 | 结果 | 场景用时 | 最终原生累计 tokens |
|---|---|---|---|---|
| delivery.basic | controlled | pass / cleanup complete | 66.30s | 无模型请求 |
| recovery.waiting-merge-restart | controlled | pass / cleanup complete | 85.03s | 无模型请求 |
| delivery.local-codex | live | pass / cleanup complete | 105.59s | 64,187 |
| delivery.orca-codex | live | pass / cleanup complete | 266.58s | 309,669 |
| recovery.local-codex | live | pass / cleanup complete | 135.03s | 113,360 |
| recovery.orca-codex | live，修复后 | pass / cleanup complete | 321.74s | 378,156 |

成功运行合计 **865,372 tokens**（包含 cached input，取最终累计样本、不重复相加）。两条 recovery 的重启前、重启后、交付/清理后 usage 不变，无新增模型回合；原项目/数据库、执行/工作流/attempt、会话/终端、工作区、当前回合和权限保持。Orca 两个真实场景 readiness/business 均 settled/succeeded，准备 verified/ready。各目录保存 summary、scenario、usage 与筛选后的身份/权限证据，不复制完整输入、dispatch capability、私有数据库或认证。

首轮 Orca recovery 失败于服务启动检查：`unsupported_schema: managed Orca resource evidence is incomplete or changed`。此前原生 readiness/business 已成功，原生累计 **380,731 tokens**，现场清理完成，独立保存在 `recovery.orca-codex-failed/`；不把局部执行算成恢复成功。

修复 `74dd59c`：旧启动检查强制非空 writable roots，与当前 full access 的空列表契约冲突。比较环境 roots 与先前已通过 codec/身份验证的原 PolicyResources roots，保留原 runtime、启动命令及资源完整性检查，workspace/full access 均拒绝 roots 变更。不新增迁移/兼容或重发路径。

无 token 定向回归：`go test ./internal/steerline -run '^(TestOrcaRestartPreflightRetainsOriginalPolicyRoots|TestManagedOrcaFrozenResourceEvidenceIsVerifiedBeforeInput)$' -count=1` 通过（54.603s）；`go test ./internal/testboundary -count=1` 通过。随后仅用新现场重跑 Orca recovery，通过后未重复其余已成功场景。前五项在 `5ebf704` 验证；最后一项在 `74dd59c` 验证，后者只改 Orca 重启 roots 检查与测试/文档。

本轮全部已取得原生报告合计 **1,246,103 tokens**（成功 865,372 + 失败 380,731），不换算账单。未运行 full Go 或重复 L2。

原始报告位于 `C:/Users/lmas1/AppData/Local/Temp/`：

- `sl-l3-all-controlled-20261002-172900`
- `sl-l3-all-delivery-local-20261002-173226`
- `sl-l3-all-delivery-orca-20261002-173533`
- `sl-l3-all-recovery-local-20261002-174034`
- `sl-l3-all-recovery-orca-20261002-174336`（失败）
- `sl-l3-all-recovery-orca-fixed-20261002-175723`（通过）
