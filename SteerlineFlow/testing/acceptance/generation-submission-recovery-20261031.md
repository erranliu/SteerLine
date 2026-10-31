# 测试生成提交取消与缺失原请求恢复

日期：2026-10-31。普通开发及用户明确授权的安装维护；源码基线 `e1968de2d8031c834bb5f35dff8a37a0a41b6a62` 加当前工作区修改。没有修改 Runner、提示词、测试清单或 CI。

## 问题与修复

SteerlineTest #101 保存了 `submission=started`，但没有 Run ID。原请求公开查询返回 `request_not_found`；停服后记录为 cancelled，却无法获得 Run 停止证明，导致重试资格一直被阻断。首次提交诊断未留存，不能把原现场的具体触发原因确认为五秒超时。

- dispatch 在已过期的调用中不跨提交边界；边界保存后、实际 Submit 调用前已经取消时，保存明确未调用的拒绝结果。
- Submit 后用脱离调用取消、最多五秒的本地保存写入实际回执或原始提交诊断；外部 CLI 调用仍受原时限约束，不自动重提未知请求。
- 正式启动在宿主持锁、项目 actor 尚未运行时，使用 Runner 公开离线接管报告收尾已取消且无 Run 的原意图。报告必须无在线实例、命名空间匹配、原请求不存在且全部 Run 的原请求绑定完整；在线缺失、同号异正文、已知 Run 丢失及不完整报告不能据此结算。原请求、Runner pin、取消事实和 Stop ID 保留。
- 共享生成名额在启动恢复之后从最终记录重建；已受理的迟到原 Run 仍提交原停止意图，不能当成未受理。

## 隔离回归

根目录：`C:\SteerlineDebug\wt-801c-generation-recovery`。原始 summary、JSONL 和 stderr 留在各目录，没有覆盖失败记录。

| 检查 | 原始报告相对路径 | 结果 |
| --- | --- | --- |
| 新增回归在修复前复现 | `diagnostics/baseline-20261031/summary.json` | 失败：回执/错误落盘、调用前取消、过期 dispatch、离线缺失原请求收尾 |
| `go run ./cmd/test-feedback -- ./internal/testmanagement ./internal/app -run 'TestTMGeneration\|TestGenerationQueue\|TestGQ\|TestPMSGeneration' -count=1` | `diagnostics/focused-20261031/summary.json` | 48 个顶层测试通过，零失败/跳过，53.52 秒 |
| 补充迟到原请求及全部离线证明反例 | `diagnostics/late-original-20261031/summary.json` | 通过；原 Run 使用其 revision 生成 stop，不被结算为不存在 |
| 固定 L1 | `test-results/L1/20261031-fix/summary.json` | 通过 |
| 固定 L2 | `test-results/L2/20261031-fix/summary.json` | 受控核心交付冒烟通过 |

以上命令均通过 `-out` 指定表中对应目录；未运行完整 L4 或浏览器全集。新增测试位于 `internal/testmanagement/generation_submission_recovery_test.go` 和 `internal/app/test_generation_absent_test.go`。`git diff --check` 通过。

## 安装维护与真实生成

证据根：`C:\SteerlineInstall\maintenance\generation-recovery-20261031`。部署包：`C:\SteerlineInstall\deployments\20261031-113712-630a2e5f`。从本工作区构建 SteerLine/Web，复用已固定的 Runner；deployment.json 如实记录源码 dirty。产品源码和新增测试的实际 SHA256 保存于 `final-result.json`。

维护前保存 `SteerlineTest-before.db` 在线一致性备份、`101-original-generations.json`、原安装程序和配置摘要。正常部署停服后保存 `offline-takeover-before-start.json`；新程序的正式启动恢复把原生成 `cd3b941f8384e35cf730dcf8ca57c6f8` 收尾为 rejected，仍保留 cancelled、原提交信封、pin 和 Stop ID。没有直接修改 SQLite 或 Runner 私有库。

随后通过认证的 application v2 `maintenance_retry` 命令重新生成 #101：

1. 第一次新生成返回了含说明文字的测试 ID，被原清单校验判为 invalid；Run 已停止，完整失败保留在 `101-first-regeneration-invalid.json` 和对应请求/响应中。
2. 同一配置的第二次生成 `1c36a63af6aa4017b27542246f21eec6` 成功，Run `ce910c53583491399b31a0b6fb6f8701` 已停止。选择 `N101-CONTENT`、`N101-SCOPE`、`demo.progress.roundtrip`、`demo.progress.retry`、`demo.progress.cleanup`；没有放宽清单校验或把失败结果改成通过。

最终 `maintenance-final.json` / `101-generation-final.json` 证明 #101 ready、全部五任务材料 ready、当前批次 1/1 成功且零失败。`final-result.json` 核对原任务记录、契约和依赖与维护前完全一致，五份项目/执行/Web 配置摘要不变，零开发候选，数据库 integrity/foreign_key 检查通过；后台、Web、三项目 ready，Web HTTP 200。生成测试安排不等于已执行其中的项目测试。
