# 项目运行环境准备要求默认折叠

2026-11-12，受测版本为 `9cefc59858e3f37d65397fc435f2d71638a3861b` 加本次前端工作树差异。

进入项目运行环境页时，「准备要求」默认折叠。用户展开后才读取正文，收起后页面刷新操作也不会重新读取正文。

报告根目录为 `C:/Users/lmas1/AppData/Local/Temp/steerline-preparation-fold/test-results`。

| 执行 | 结果与证据 |
| --- | --- |
| `npm --prefix web test -- src/features/environment-checks/project-environment.test.tsx` | 30 项通过，覆盖初始折叠、展开后读取及协议段隐藏。 |
| `npm --prefix web run build` | TypeScript 检查和生产构建通过。 |
| `npm --prefix web run test:e2e -- environment-overview.spec.ts configuration-navigation.spec.ts -g 'all environment readiness\|runtime environments keep row actions' --global-timeout=125000` | Edge 2 项通过，覆盖展开、再次收起和任务导航。追踪位于 `browser/5272ec01988e4842b3bd2acc885e2268`。 |
| `go run ./cmd/test-feedback -level=L1 -out <目录>` | 通过，报告位于 `L1/32e7707c30df4f6e8b8a78a47e14573d/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <目录>` | 受控冒烟通过，报告位于 `L2/5f0f8c104d714d39bcd381c1b2b39656/summary.json`。 |

首次沙箱执行遇到 Edge 启动失败及 L1、L2 报告原子写入被拒绝。上表记录独立目录中的补验结果。未部署。
