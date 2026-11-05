# 环境操作具体错误展示（2026-11-03）

SteerlineTest 的旧 manifest4 材料导致重新检测被拒绝，后端回执包含 `environment.md requires one steerline-environment JSON block`，环境页面却只显示“操作未完成，请查看操作结果或刷新重试”。这是错误展示遗漏：Workspace 仅在非环境页面渲染 error，精简操作列表也未显示 diagnostic 或网络错误。

本次始终在操作提示中显示现有 error；精简环境操作结果在非 applied 状态优先显示原回执 diagnostic，缺少该字段时显示原请求的 error。成功结果保持简洁。保留原请求身份、查询、重试与幂等逻辑，不改变提交或对账规则，不改提示词、规则材料或测试定义。

受测来源为 SteerLine `195a411a0875300bb9514ff584307ce5f05d6f9d` 加本次四个 Web 文件差异。代码位于独立分支 codex/environment-error-details；先前 Runner 升级核验修复由独立 PR #439 提交，Runner 停止证据修复由 SteerRunner PR #94 提交。

证据根为 `C:\SteerlineDebug\environment-v5-20261103`：

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| Workspace 与 Operations 定向 | 13 项通过；覆盖后端拒绝、未知结果、网络错误、关闭提示以及原请求查询 | 当前工具执行记录；同组完整纳入 Web 全集 |
| 固定 L1 | passed/complete | `steerline-L1/summary.json`、`managed-l1.minimum.json` |
| 固定 L2 | passed，cleanup_complete=true | `steerline-L2/summary.json`、`managed-smoke.delivery.json` |
| Web 单元全集 | 266 项通过、零失败/跳过，complete=true | `steerline-web-unit/summary.json`、Web 原始日志 |
| TypeScript 与生产构建 | `npm --prefix web run build` 通过 | 当前工具执行记录；构建文件属于忽略目录 |
| 文档和差异 | 本次链接检查及 git diff --check 通过 | 当前工具执行记录 |

本次页面变更按用户要求提交 PR，不覆盖正在执行正式检测的安装版。SteerlineTest v5 材料已在独立项目 worktree 更新、验证并发布到其 main；真实重新检测结果由该项目专属验收记录保存。
