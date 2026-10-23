# 近期活动展示优化验收

2026-10-20 合并后补验：三份相关 Web 测试 26 项和生产构建通过；SQLite 原事实读取测试修正为 PR #356 已确定的英文 `implement.report`，中文状态/诊断/噪音和绑定断言保留。定向反馈 22 项（含子用例/选择器）全部通过、零 skip，证据为 `C:/Users/lmas1/AppData/Local/Temp/steerline-merged-acceptance-20261020-125058/focused-fixes/summary.json`。仅修正旧测试，不改产品文案。RAD-M01 保持人工待验，[操作导航](merged-pr-regression-20261020.md#执行页与测试结果可读性)区分历史截图和当前交互体验。

范围：用户确认的“执行与待处理 → 近期活动”排序与文案优化。设计规则见 [Activity 契约](../../../docs/product-design/activity.md)。本次为普通开发，不占 roadmap Task ID。

## 自动验收（2026-10-19）

环境：Windows、本工作树隔离的 Go/React 测试夹具；源码基线 `0dbddc8d9ce964545723d8be3cbc85ba0cd7840c` 加本次未提交修改。

| ID | 验收项 | 验证入口 | 结果 |
| --- | --- | --- | --- |
| RAD-A01 | 近期活动和折叠内部事件均按 seq 最新在前；相同时间不改变顺序；刷新后新活动居首且不修改查询缓存原顺序 | `runs.test.tsx` 的 recent changes 场景 | 通过 |
| RAD-A02 | Workflow 节点保留英文；开始、切换、状态变化和诊断变化使用独立文案；Workflow 与状态同时变化时只附目标状态 | `TestRunnerActivityTransitionDisplay` 的五种变化 | 通过 |
| RAD-A03 | 状态/诊断记录显示自身历史 Workflow；诊断原因保留；技术事件默认折叠 | 同一 React 场景及 Go 测试的原事实断言 | 通过 |
| RAD-A04 | 保留规范变化字段校验及事件身份；客户端类型检查和生产构建成功 | `TestStateNamingActivity`、Web build | 通过 |

执行命令与证据：

- `go run ./cmd/test-feedback -- ./internal/product/query -run 'TestRunnerActivityTransitionDisplay|TestStateNamingActivity' -count=1`：通过，11 条测试记录（含父测试和子测试），零跳过。原报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261019T083618.516060900Z/summary.json`。
- `npm --prefix web test -- src/features/runs/runs.test.tsx`：最终 8/8 通过，无未处理异常。首次新增刷新断言未等待查询通知，补为 `waitFor`；随后发现原夹具把任务页返回给 `/presentation`，补齐该测试的 presentation 缓存后通过。
- `npm --prefix web run build`：TypeScript 与 Vite 生产构建通过。
- `git diff --check`：通过。

仅选择 L1 定向验证与构建；本次不修改执行节点、异常、重试或恢复行为，无需 L2/L3/L4、真实模型或 PR 流程。未启动调试服务，未使用安装环境。

## 人工验收

RAD-M01：待用户在更新后的 Web 中查看近期活动，确认最新在前、英文 Workflow 与中文状态易读；参与窗口为用户后续体验，不阻塞本次源码交付。当前未记录真实浏览器视觉验收通过。
