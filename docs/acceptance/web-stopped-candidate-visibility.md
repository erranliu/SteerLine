# 已停止参与候选的显示筛选验收

归属：2026-10-20 普通开发“已停止参与候选默认隐藏，增加显示勾选项”。源码基线 `f21aea1fa972f9a89ff5fd6515b0eba4edf229ce` 加本次工作区变更。

## 行为

执行页默认隐藏 `participation=abandoned` 的候选，工具栏提供默认未勾选的“显示已停止参与候选”。候选总数和任务上的候选数量按当前显示范围计算。取消勾选时，当前已退出候选切换为同任务首个可见候选；全部隐藏时提示勾选查看历史。停止操作仍先核实原请求回执和权威快照，再自动选中剩余参与候选的最大编号。

## 自动验证

环境为 Windows、Vitest/jsdom 及锁文件依赖；复用现有隔离组件夹具。仅修改显示与选择行为，选择定向 L1 组件回归和构建，不涉及服务、Runner 或交付事务，因此未执行 Full/L2/L3/L4。

| ID | 范围 | 结果 |
| --- | --- | --- |
| SCV-A1 | 已有退出候选默认隐藏、勾选显示历史、取消勾选隐藏并切换当前详情、显示数量随筛选调整 | 通过；扩展现有 `runs.test.tsx` 停止参与场景 |
| SCV-A2 | 原停止请求身份、快照先到仍等待原回执、成功后选择最大剩余编号、不抢用户后续选择，以及历史只读 | 通过；现有 Runs/状态/任务详情定向回归 |
| SCV-A3 | TypeScript 检查与生产资源构建 | 通过 |

证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-candidate-visibility-39560c8e571a44bfb416bca9acbcdac1`。

- `npm --prefix web test -- src/features/runs/runs.test.tsx src/features/runs/state.test.ts src/features/tasks/task-detail.test.tsx`：3 文件、18 项通过，最终日志 `web-tests-final.log`。
- `npm --prefix web run build`：TypeScript/Vite 通过，最终日志 `web-build-final.log`。

未启动调试环境或操作安装目录；未进行真实浏览器人工验收。自动验证仅证明上述组件行为和构建结果。
