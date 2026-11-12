# 丢弃执行部署的环境提交回执验收

2026-11-12。受测源码为 `49d290716106fdc80bef94fc0f36f439b6b80848` 加本次工作区修复。308 项目环境检测 `4ee2990b037c4f409ba1fa68725e54b8` 的第 1 轮没有本地 Run ID。原 Runner 在离线查询请求 `77b23682805a4d13f0aadb60d4a7b5d5` 时返回 `request_not_found`。原失败报告为 `C:/Project/SteerLine/bin/deploy-timings/3be605fecdaa43c7ba9fde51048ed4d0.json`。

丢弃流程按原请求 ID 读取离线回执。回执指向 Run 时核对项目、活动和轮次，再用 Run ID 和输入摘要匹配已结算的资源清单。回归覆盖请求不存在、准备中、持久拒绝、已接受、归属不符、Run 缺少清单证据和查询不可用。

## 验证

隔离证据根为 `C:/SteerlineDebug/wt-9203-discard-environment`。

| 命令 | 结果 | 证据 |
| --- | --- | --- |
| `go run ./cmd/test-feedback -level=L1` | 通过 | `test-results/L1/41d5abe11d1d4f69915ff159c92526b7/summary.json` |
| `go run ./cmd/test-feedback -level=L2` | 通过 | `test-results/L2/41d5abe11d1d4f69915ff159c92526b7/summary.json` |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment` | 通过 | `test-results/L4/393a38b7cea64f8e9e7613827b894140/summary.json` |
| `go run ./cmd/test-feedback -- ./internal/app -run '^TestDeploymentDiscardRuns$/^offline-flow$/^environment-missing$' -count=1` | 通过 | `diagnostics/b00cc668c20545bbb299e74894237314/summary.json` |

从修复工作区执行 `build-deploy-discard-runs.bat -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -NoPause`，退出码为 0。停机验证、Runner 升级预检和安装文件摘要校验通过，后端、Web 和三个项目均就绪。部署记录、阶段时间和停机证据在 `C:/SteerlineInstall/deployments/20261112-064242-a5c7031c/`。

旧项目库、应用状态及 Runner 状态已归档到 `C:/SteerlineInstall/maintenance/discard-runs-60f737d5e66d48bbb4b39d100ed0e0a5/`。逐库只读核对确认新库包含相同的 20 个任务编号和 8 条依赖，全部任务为 `open`，三个项目均为 `manual`，环境活动与 Runner candidate 表为空。
