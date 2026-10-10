# 任务列表与流程状态可读性验收

仅归属本次普通开发，[Spec](../spec/console-flow-state-readability.spec.md)、[Plan](../spec/console-flow-state-readability.plan.md)。不改变前项验收归属或 roadmap。

| ID | 自动验收项 | 结果 |
| --- | --- | --- |
| CRD-A1 | 基于事实的任务阻塞提示、原状态保留、单行固定列、大量任务、筛选及选择/焦点/离线 | 通过：console 全包、事实阻塞/筛选及对齐定向回归 |
| CRD-A2 | 流程/状态拆分、当前值、技术变化隐藏、完整详情、五条可见消息及宽窄渲染 | 通过：console 全包及最终渲染样本，160/120 列可读完整变化，80 列当前流程/状态可见并可滚动 |
| CRD-A3 | 技术/工具噪声不挤掉可读变化、原绑定/事务/顺序/预算和原始记录保持 | 通过：SQLite/query 全包，追加技术噪声与大正文预算定向回归 |
| CRD-A4 | vet、构建、文档导航和 diff 检查 | 通过：受影响包 vet、构建、文档链接和格式检查 |

环境使用现有隔离测试夹具，未启动模型或安装版。没有新增人工验收窗口；渲染样本随最终证据记录。

## 2026-10-10 实现与验证

证据目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-console-readability-20261010-151519/`。普通开发分支 `codex/console-flow-state-readability`，接续已合并的 PR #278，不修改原持久化和生产事务。

- `console.log`：console 3.959s、activityrender 0.818s 通过。补上同一诊断去重和当前流程/状态单行后，`console-final.log` console 2.285s 通过。最后前置原因缺失的原事实展示增量见 `console-blocker-final.log`。
- `render.log`：36 个任务、真实截图同类 publish/blocked 问题，160x40、120x30、80x24 样本通过；检查每行宽度与总高度、当前流程/状态可见、宽屏一份诊断且流程切换可读。80 列依原 Tab 聚焦反馈，历史可用原滚动键读取。
- `query.log`：SQLite 10.756s、query 0.768s 全包通过。`budget.log`：定向 TestPS1RecentActivityReadsLatestTenFromExactAttempt 通过（1.061s）；二十条技术变化和工具活动后仍返回五条可读变化，再追加五条各 400000 字节的技术诊断，原 1 MiB 正文预算仍优先保留可读变化。
- `vet.log`、`static.log`：受影响包 vet、构建及 319 Markdown/1939 导航链接检查通过（0 错误）。最终增量重新静态检查见 `static-final.log`；构建产物为证据目录 `steerline.exe`。

首轮回归发现旧断言要求普通活动占默认反馈区，已按用户确认的两信息投影更新，同时断言旧结果和通用活动不挤入主视图；任务已有执行时采用原 TaskStatus，避免旧启动阻塞误导。没有放松命令资格、目标绑定、答案提交和迟到响应检查。
