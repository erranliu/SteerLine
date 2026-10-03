# 测试依赖边界验证

2026-10-02（Asia/Shanghai），基于 #180 的 `8b1094d` 工作树验证本次解耦。生产源码、go.mod/go.sum 未修改；新增代码仅为测试适配层、边界检查及 runner 的 L1 登记。

| 验证 | 结果 | 证据 |
|---|---|---|
| `go test ./internal/testboundary ./tests/e2e/internal/... ./cmd/test-feedback -count=1` | pass | 下列定向说明；工具命令输出 |
| L1 固定最小入口 | pass，complete=true，执行约 29.36s | `l1/summary.json`；quick-rules 包含 internal/testboundary 并实际通过 |
| L2 smoke.delivery，controlled | pass，cleanup_complete=true，执行约 69.78s | `l2-controlled/summary.json` 和场景元数据；coverage=gate-only |
| L3 recovery.waiting-merge-restart，controlled | pass，cleanup_complete=true，执行约 82.63s | `l3-recovery-controlled/summary.json` 和场景元数据 |
| 全部 E2E 场景编译 | pass；最后 usageEvidence 局部变量更名后复核通过 | `go test -tags=e2e ./tests/e2e/live ./tests/e2e/smoke ./tests/e2e/suites/... -run '^$' -count=1` |
| sandbox-setup 工具包单测 | pass；没有执行初始化工具 | `go test ./tests/e2e/internal/... ./tests/e2e/cmd/sandbox-setup -count=1` |

边界回归检查真实生产依赖闭包和启用 e2e tag 的 imports/TestImports/XTestImports（包含 helper）。合成图验证 app 经中间包引用 fixture 被拒绝；场景和 testenv 直接导入生产 facade/原生实现被拒绝，数据合同和指定适配层可用。fixture 仅作为库存入口存在，不会因此被误报成生产依赖。

evidence 回归核对正常模型/权限/原会话，拒绝不同 project、execution、attempt、generation、config、profile 接受版本；缺少实际权限资源不能只凭配置通过。Orca 原 runtime、两回合成功、准备 verified 保留，缺失准备、失败回合和错误身份被拒绝。native 回归核对固定模型/effort、Orca 原启动参数、库存 ownership 字段以及截断/原生拒绝的失败路径。

原始目录分别为 `C:/Users/lmas1/AppData/Local/Temp/sl-boundary-l1-20261002-164839`、`sl-boundary-l2-20261002-165116`、`sl-boundary-l3-20261002-165326`。本目录只复制 summary 与场景元数据，不复制完整输入、数据库或认证。

没有跑 full 或真实 L2/L3，没有调用模型或创建外部测试 PR。受控报告的 source_commit 是验证前基线，worktree_status 记录待提交的解耦改动；验证后仅补充本文档，最终局部变量更名仅作编译复核。
