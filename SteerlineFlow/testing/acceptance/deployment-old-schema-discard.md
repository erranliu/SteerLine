# 显式放弃旧运行时重建任务库

普通部署核验和启动在读取数据库时继续校验当前完整 schema。选择 `-DiscardRuns` 后，部署可只读打开任务图和停机核验所需表定义仍兼容的旧库。原宿主和 Web 退出、项目锁、项目和 Runner 归属、旧 Run 资源结算仍经部署入口核实。旧 Roadmap 文档批次在父记录上绑定的 Run 也计入结算。

部署用正式任务入口在当前 schema 重建任务编号、内容、固定契约和依赖。原项目库、应用与 Web 状态及 Runner 状态按现有切换记录归档。新库所有任务均为 open，项目均为 manual 且暂停派发。

## 验证

受测基线 `7735f44d4265452e9737c73b64ea713db44f7bbd`，包含本次未提交变更。原始证据根为 `C:\SteerlineDebug\wt-81eb-discard-schema`。

Go 测试使用 `go run ./cmd/test-feedback` 执行，`-out` 指向对应报告目录。

| 参数 | 结果 | 报告 |
| --- | --- | --- |
| `-level=L1` | 通过 | `test-results/L1/schema44-20261111/summary.json` |
| `-level=L2` | 通过，清理完成 | `test-results/L2/schema44-20261111/summary.json` |
| `-level=L4 -suite=go -scenario go.storage` | 通过 | `test-results/L4/storage-schema44-20261111/summary.json` |
| `-level=L4 -suite=go -scenario go.deployment` | 通过 | `test-results/L4/deployment-schema44-20261111/summary.json` |
| `-- ./internal/app -run '^TestDeploymentDiscardRuns$' -count=1` | 通过 | `diagnostics/discard-initial/summary.json` |

定向夹具移除 schema 45 新增的三个表及关联索引，验证 schema 44 的归档路径、原库字节不变、任务及依赖重建、新库暂停派发，以及普通读写拒绝旧库。覆盖旧 Roadmap Run 缺少结算、表定义变化、未知或过新版本、原宿主身份错误的阻断。

Windows `scripts/test-deployment-stop.ps1` 和 `scripts/test-build-deploy-pipeline.ps1 -OutDir <证据根>/diagnostics/powershell-deployment/pipeline` 通过。流水线共 15 个隔离场景，原日志位于 `diagnostics/powershell-deployment/`。

本次代码验收使用隔离现场。实际安装的 schema 44 恢复发生在本次改动之前，未用该安装作为回归测试现场。
