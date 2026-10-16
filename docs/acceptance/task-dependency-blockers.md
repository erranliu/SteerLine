# 执行页面任务列表的依赖阻塞展示

普通前端调整，范围为“执行与待处理”左侧任务列表及未启动任务的空面板提示，不修改任务业务状态或启动资格。

待执行任务的 `dependency_ready=false` 时，列表显示琥珀色“依赖阻塞”徽标、左侧标记和锁图标，并列出尚未交付的前置任务编号。阻塞任务仍可选中查看详情；依赖解除后恢复“待执行”。执行中、已交付、已放弃任务保留原状态，不被首次启动依赖事实误标。

| 验收项 | 结果 |
| --- | --- |
| TDB-A01：区分依赖阻塞，列出未交付前置，保留选中与点击行为；解除后清除提示 | 通过，组件测试 1 项 |
| TDB-A02：执行中、已交付和已放弃任务不被误标阻塞 | 通过，组件测试 3 项 |
| TDB-A03：TypeScript/Vite 构建 | 通过 |

在 `web` 目录执行 `npx vitest run src/features/runs/run-task.test.tsx --maxWorkers=1`，4 项通过；`npm run build` 通过。证据为 `.evidence/task-dependency-blockers/web-tests.log`、`web-build.log`。本次使用隔离组件 fixture，不启动调试服务、模型或安装环境；未执行真实浏览器视觉验收，不跑 full/L2/L3。
