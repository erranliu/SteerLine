# Spec：候选观察界面

任务 ID：28；任务名称：候选观察界面；目标：`PHASE1-CANDIDATES`。配套 [Plan](candidate-console.plan.md)。状态：待实现与验收。一个独立实现 PR 交付。

## 范围与前置

硬前置：#27。传递包含 #26。消费[候选模型](task-candidates.spec.md)和[交付仲裁](candidate-delivery.spec.md)已交付的 Query/Command，遵守[共同约束](phase1-delivery-constraints.md)。本任务改造现有终端控制台与 CLI 展示；WinUI 仍归第二阶段任务，不新增图形界面框架或业务写者。

结构重构安排拟追加 #35 为中心开工前置；取得补边回执后消费[中心包](center-boundary.spec.md)提供的同一 Query/Command。原 #27 前置和已创建任务的固定界面范围保持。

## 展示与操作

任务列表每个 Task 一行，按中心事实显示任务状态、候选数、待处理数、交付结果与未结算资源提示。任务详情可展开全部候选并稳定选择 workflow_run_id；每候选独立显示阶段、配置/后端/目标、base/worktree、问题、PR、胜出/落选/冲突及资源结算状态。Task closed 但落选执行仍活跃或 unknown 时，在需处理和历史入口均可找到；不能只保留胜出候选。

候选列表按服务分页加载，刷新后按 ID 保留选择、展开、阅读位置与跟随设置。自动选择仅在尚无用户选择时发生；迟到消息不能切换到兄弟候选。当前候选变更须清晰显示原内容归属，正文、轮次和问题缓存以完整执行身份索引。没有精确 turn/phase 能力时显示执行级输出和缺口。

single Task 沿用原开始流程；multiple Task 提供明确的“新增候选”，显示固定 Task/base/契约与所选配置，通过 #26 的 start_candidate 提交。回答、重试、恢复只作用于所选候选或其精确 Decision。Task 级操作与执行级操作使用不同目标；不可用操作显示中心错误原因，不自动挑一个候选、不提供强制胜出或远端取消。

提交前保存原请求信封和身份，重复点击、超时、退出重开按原 ID 查询/补交；不能刷新 expected_version 后静默重发。问题答案平台接受、开始续跑及 applied 证明分别展示。PR 打开精确候选 URL，人合并后等待中心核验；点击或浏览器退出不变更 Task。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| CC1 | 三候选任务只有一条 Task 行，候选分页完整；各阶段/问题/PR/资源与同一服务快照一致 |
| CC2 | 切换候选、列表重排、迟到输出和重启不串正文/答案，保留所选候选及阅读位置 |
| CC3 | 新增候选、回答、恢复提交精确身份；重复点击和丢回执复用原请求；stale/ambiguous/unsupported 清楚显示 |
| CC4 | Task 已交付而落选执行 unknown/PR 关闭失败时仍能定位待办，不能以落选标签伪称执行停止 |
| CC5 | Windows 原生终端中完成键盘导航、中文输入、窄窗口与滚动检查；截图与服务回执对应 |

CC1–CC4 可使用真实中心加可控 worker/PR 替身，CC5 必须使用真实 Windows 控制台。跨主机 agent 和真实 PR 合并验收归 #29。本任务不得将替身 PR 标记为生产通过。证据路径为 docs/validation/candidate-console/。
