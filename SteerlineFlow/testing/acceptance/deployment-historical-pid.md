# 历史本机 Orca PID 阻塞升级（2026-11-05）

受测源码为 `aa08dfee9042f2be423f0999d7e75dab3fa66286` 加本次工作树差异。真实失败包位于 `C:\SteerlineInstall\deployments\20261105-021136-f588ceb2`。其 `shutdown-verification.json` 记录正常停机。通过原 Runner 公开离线接口确认，故障 Run `9e085b813e3f039549f1108ea5206065` 已关闭、停止操作为 confirmed，原 Runner 停机回执的 `resources_settled` 为 true。重新查询 PID 26628 返回 Access is denied。

升级预检和换版后的启动绑定仅在原构建的停机结算回执有效、Run/input 和本机 Orca Codex 证据范围一致、停止操作已确认时采信这份证据。其余资源诊断和业务终态、输入、未完成控制继续核验。回归覆盖原故障和活进程、缺回执、错构建、停机未结算、缺停止操作、错范围及畸形 Run 集合。

验证证据根为 `C:\SteerlineDebug\wt-083e-deployment-fix-886a04e5`。尝试对实际安装执行修复版只读预检时，安装版已经重新启动，预检按服务运行状态阻断。本次实现保留旧运行数据。

| 命令 | 结果和原始证据 |
| --- | --- |
| `go run ./cmd/test-feedback -out <目录> -- ./internal/runnerclient -run '^TestUpgradeDrainedUsesCandidateProofAndBothBusinessReaders$' -count=1` | 通过，`diagnostics/runner-upgrade-regression/summary.json` |
| `go run ./cmd/test-feedback -level=L1 -out <目录>` | 通过，`test-results/L1/minimum/summary.json` |
| `go run ./cmd/test-feedback -level=L2 -out <目录>` | 通过，`test-results/L2/delivery/summary.json` |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment -out <目录>` | 升级组通过，应用组因 `seedStoppedPublicRun` 夹具缺少结果文件能力失败，见 `test-results/L4/deployment/summary.json` |
| 基线 `go test ./internal/app -run '^TestDeploymentInterruptedRecovery$' -count=1 -json` | 在相同夹具调用复现上述错误，见 `diagnostics/baseline-aa08dfee/test.jsonl` |
