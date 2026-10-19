# Runner 兼容接管 Plan

对应 [Spec](../spec/service-generation-recovery.spec.md)；采用既有 [worker-lifecycle](../product-design/worker-lifecycle.md) 和 caller outbox，不新增 lifecycle、持久兼容 manifest 或全局 hold/release。实际记录见[验收](../acceptance/service-generation-recovery.md)。

实施顺序：runnerclient 增加公开 takeover 与裸 startup stops；application 在共享 Runner 启动前采样项目 caller Store、保存原停止信封；按原 Run/input/request digest 重绑当前服务 pin；环境检查沿原 ID/报告和旧 selection 取证；部署分别验证旧服务退出与新候选兼容。所有原任务、候选、结果、PR 和 Delivery 都保留原事实。

## 测试计划

| 验收 / 级别 | 场景、环境及命令 | 时点 / 通过条件 / 证据 |
| --- | --- | --- |
| SLC-A01 / L1 | Windows 隔离进程 CLI，`go test ./internal/runnerclient -count=1 -timeout=3m` | 接线后；错候选 SHA/目录/contract/unsupported/重复 item 拒绝，原 stop 裸信封不变、明确 revision 拒绝不晋升 unknown；专属验收 |
| SLC-A02 / L1 | 隔离 Git/SQLite/公开 CLI，`go test ./internal/app -run 'TestSharedRunner\|TestTakeover\|TestDeployment' -count=1 -timeout=5m` | 接管后；证明才能更新当前 pin，原 candidate build/input/PR/unknown 保留；错原绑定、活动旧服务、本地未关拒绝；专属验收 |
| SLC-A03 / L1 | 同上，`go test ./internal/app -run '^TestStartupCancellationOutbox' -count=1 -timeout=120s` | 启动前；原 StopRequested 的首次信封先持久保存，第二次收集字节相同，不启动 Runner，不改原 submit/Run/pin；专属验收 |
| SLC-A02/A03 / L1 补充 | `go test ./internal/app ./internal/environmentcheck -run 'TestEnvironmentCheck\|TestStartupCancellationOutbox\|TestSharedRunnerCheck' -count=1 -timeout=4m` | 旧 Check 传输接线后；旧 EXE 不存在仍能在逐项证明后用当前 CLI 读取，原 selection/resource digest 保留，错 input digest 拒绝；专属验收 |
| SLC-A04 / L1 | Windows PowerShell，`powershell -NoProfile -File scripts/test-deployment-stop.ps1`、PowerShell Parser | 部署脚本修改后；保留原 service 退出与 resource=false，跨构建必须另有正确 candidate/takeover 证明，失败不替换；专属验收 |
| SLC-A05 / Controlled 组合 | 复用 Runner T03/T06 的原正常换代现场，通过本仓公开 client 恢复原 candidate/投影；不额外创建开发任务或真实合并 | 真实联调现场可用时；Task/Candidate/Run/session/PR/Delivery 不变、零重复 submit/resume；当前未执行，不能由上述 L1 冒充 |

Runner 的真实 Windows EXE、原 owner/parser/离线审计 checkpoint 与 startup stops 由其对应受控场景验证；此仓不复制原生状态机测试。当前不跑 full/L2/L3，不操作安装版或建立新真实 PR；仅待组合现场项和原生接口资格限制整体完成。
