# SteerLine 等待合并重启回归

当前仅登记 L3 controlled `recovery.waiting-merge-restart`：公开 CLI fixture 保持原 Run/PR，重启 SteerLine，核对候选、原 request/body 与投影、阻塞后继，随后合并并提交唯一 TaskDelivery。重启不制造新 submit/reply/resume。

```powershell
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.waiting-merge-restart'
```

Codex/Orca 原生断连、终端身份、未知输入、同 thread 恢复、缺 final/补报、资源资格属于 SteerRunner 专项。调用方未知回执、原授权 revision、事件游标与停止义务由 SteerLine 对应 owner 测试承担。完整本机 Codex 运行和真实 PR 闭环继续由 L2 覆盖。

[精简 Spec](../../../../docs/spec/runner-test-convergence.spec.md) · [验收](../../../../docs/acceptance/runner-test-convergence.md) · [E2E 指南](../../README.md)
