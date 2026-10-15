# SteerlineTest debug task plan

Read the bound Spec. Replace result.txt with the marker and actual branch, verify and commit only that file, then stop. The separate report input formats existing facts without tools. Run publish.ps1 only after a separate publication input and preserve failures honestly. Existing external login remains outside this package.

## 测试计划

最小本地检查：在实现阶段读取 result.txt，逐字核对 marker、当前分支和末尾换行，检查本次提交只修改 result.txt；退出码为 0 后才报告成功。发布阶段运行固定 publish.ps1 并核对返回的专属 base/head/PR URL。合并后从 SteerLine 产品查询核对唯一交付与后继 ready、未自动启动，证据记录于本包 evidence。

SteerLine 开发侧的长期自动入口为完整 `go run ./cmd/test-feedback -level=L2`，其独立 SteerlineTest fixture 覆盖真实本机 Codex 三阶段、Git/PR 合并和 TaskDelivery（[RTC-A3](https://github.com/erranliu/SteerLine/blob/main/docs/acceptance/runner-test-convergence.md)）；本模板发布脚本另由 `go test ./tests/e2e/internal/driver -run '^TestDebugPublicationReusesPR$' -count=1` 验证隔离 Git 推送与 PR 复用，调试包模拟/reset 由 test-debug-release.ps1 验证（RTC-A2/A4/A6）。两种 fixture 的产物分别核对，不把 L2 的固定 marker 当作本模板的分支产物。普通模板修改不默认运行 Full 或原生专项矩阵。沙箱、token、原生审核与恢复的能力验收在 SteerRunner 项目维护，不从原生 rollout 取证。
