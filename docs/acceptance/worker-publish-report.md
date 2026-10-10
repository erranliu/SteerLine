# 发布结果补报验收

归属单一普通开发事项：[Spec](../spec/worker-publish-report.spec.md)、[Plan](../spec/worker-publish-report.plan.md)。

| ID / 类型 | 验收内容 | 状态 / 证据 |
| --- | --- | --- |
| WPR-A1 / 自动 | 合法 publish 直接处理；普通文字/缺失结果绑定原发布补报一次；补报失败不重试发布 | 自动通过；见下方证据 |
| WPR-A2 / 自动 | 原失败、未知、工作区/PR 不匹配不升级；恢复/重复原结果不自动重投 | 自动通过；见下方证据 |
| WPR-A3 / 自动 | 发布报告名称、原发布正文查询及历史范围 | 自动通过；见下方证据 |
| WPR-A4 / 自动 | 隔离 Git/SQLite/controlled app-server 三/四轮流程及同会话、Attempt、重启不重投 | 自动通过；见下方证据 |
| WPR-A5 / 自动 | publish 输入要求结构化 `outputs.pr_url`；合法成功结果核验 PR 后跳过补报，缺字段继续补报 | 自动通过；见下方发布结构化 URL 验证 |

无独立人工项；不操作 C:\SteerlineRuntime 中 103 任务。报告仍沿用原生权限，前后事实比对不是强外部写入隔离；本次未执行真实模型/GitHub 发布。

## 自动验证证据（2026-10-10）

全部为 Windows 隔离夹具，0 失败、0 skip；测试对应 Plan 的验收项映射。

| 验收项 | 执行 / 结果 | runner 证据目录（含 summary.json、go-test.jsonl） |
| --- | --- | --- |
| WPR-A1、A2 | execution 分步规则定向回归：37 项通过，277.035 秒 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T071602.487068100Z` |
| WPR-A1、A2 | 新增发布补报与更新提示词检查：11 项通过，100.502 秒 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T071914.068561800Z` |
| WPR-A2 | `TestWorkerStagePublicationReportReadRetrySettlesOpenPR`：1 项通过，19.231 秒；补报后 provider 暂不可用，随后只读对账绑定原 PR，无新输入 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T072129.724154100Z` |
| WPR-A3 | console 定向回归：10 项通过，3.431 秒 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T072019.862271000Z` |
| WPR-A4 | `TestWorkerStageLocalRoundTrip`、`TestWorkerStageLocalPublishReport`：2 项通过，213.489 秒；原三轮/补报四轮、来源与候选、同会话/Attempt、重启不重投通过 | `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T071835.549153700Z` |

集成测试首次编译发现测试断言中的 int/uint64 类型不一致，已修正；上表为修正后的通过证据。未运行 full/L2/L3/live，原因见 Plan。

## 主干冲突解决验证

合入 `origin/main`（`3e6d4fde`）后，保留主干 blocked 显示“阻塞”与本任务 publish.report 显示“发布报告”。`go run ./cmd/test-feedback -- ./internal/console -count=1` 全部 173 项通过，0 失败、0 skip，5.398 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T072928.625370300Z`。未改变 execution 发布补报规则。

## 发布结构化 URL 验证（2026-10-10）

WPR-A5：`TestWorkerPrompt*`、`TestWorkerStageTransitions`、`TestWorkerStagePublishReport` 定向回归 15 项通过，0 失败、0 skip，129.373 秒。验证 publish 与 publish.report 均要求 done 返回 `outputs.pr_url`；合法 publish 查询实际 PR 后直接等待合并，只有三轮输入；URL 仅在 summary 中仍补报一次，原补报保护通过。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T084930.566618700Z`（summary.json、go-test.jsonl）。

直接路径断言改为并发安全计数并补充实际绑定 URL 后，最终版本重跑 `TestWorkerStageTransitions`：1 项通过，0 失败、0 skip，19.272 秒。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T085041.429649500Z`。`git diff --check` 通过；未操作安装环境或既有任务，不运行 full/L2/L3/live。
