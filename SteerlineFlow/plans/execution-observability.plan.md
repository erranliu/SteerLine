# Runner 执行展示与最近消息 Plan

用户已确认后直接实施，不纳入 roadmap。[验收](../testing/acceptance/execution-observability.md)。遵循 [Worker 生命周期](../../docs/product-design/worker-lifecycle.md) W11–W14/V04/V06 与 [Activity 契约](../../docs/product-design/activity.md)，本次不改变执行/恢复状态机。

## 实施

1. Runner 提供原子 `workflow_changed.transition.from/to`；SteerLine 原样持久化公开 event，在 Activity 投影生成可读流程/状态变化及阻塞原因。Recent 查询保留最近五条变化、十条技术事件并恢复原 seq 排序，共用 1 MiB 预算。技术事件折叠进详情；不补画缺失历史。
2. 新增 `/runs/<candidate>/last-message?execution_id=...&attempt_id=...`。HTTP 和应用层核验项目/候选/Run/Attempt，应用只调用固定 EXE 的公开 `last-message` CLI；不直接读 Runner DB、私有 rollout 或恢复原生会话。
3. Web 当前执行详情独立展示最后一条已观察摘要、观察时间及距今时长。仅该组件在线且页面可见时每 3 秒查询；切换执行/Attempt、离开执行页或浏览器后台时取消请求。query key 固定 home/project/instance/candidate/Run/Attempt；旧响应不能覆盖新目标。
4. 不因轮询成功更新消息时间；错误保留最后样本并提示读取失败；无样本如实展示。采集为最佳努力，长时间无新样本不推断卡死。Runner 原观察流程正常运行，不增设所有执行的消息扫描或定时查询。

## 测试计划

| 自动项 | 级别 / 场景 / 环境 | 命令 | 时点 / 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| EOV-A01 | 定向 Go / SQLite 隔离夹具；`TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding`，含内部噪声子场景 | `go test ./internal/storage/sqlite -run '^TestRunnerPublicReads' -count=1 -v` | 提交前；五条变化不被四十条技术事件挤掉；中文语义及 HTTP401 原诊断可读；原 final/Run/Attempt 边界保持 | acceptance EOV-A01，`.evidence/observability` |
| EOV-A02 | 定向 Go / 真实本地 HTTP 和隔离 reader；`TestClientHTTPRoutes`、统一宿主路由测试；runnerclient 契约 | `go test ./internal/product/httpapi ./internal/product/applicationhttp ./internal/product/client ./internal/runnerclient -run 'TestClientHTTPRoutes|TestUnifiedStartupAuthenticationRoutes|TestSteerRunner' -count=1 -v` | 接入后；错误目标零源调用；重复读保留源时间；新路由转发、原 CLI 认证/摘要固定不变 | acceptance EOV-A02，同上 |
| EOV-A03 | Web Vitest / jsdom / fake timers；last-message、runs、queries | `cd web; npm test -- src/features/runs/last-message.test.tsx src/features/runs/runs.test.tsx src/shared/api/queries.test.ts` | UI 完成后；单选轮询、切换/后台/卸载取消、迟到响应隔离、错误保留、源时间不变、原活动/最终回复 Tab 刷新保持 | acceptance EOV-A03，同上 |
| EOV-A04 | Web 类型及生产构建；三 EXE 编译 | `cd web; npm run build`；两仓各按当前 build 包路径编译三 EXE 到 `.evidence/observability` | 提 PR 前；编译成功，不部署安装目录 | acceptance EOV-A04，同上 |
| EOV-A05 | 两仓公开接口闭环 / Runner 实际 EXE 与受控常驻服务；ROV-A03 | Runner `go test ./tests/integration -run '^TestSRRunWorkflow$' -count=1 -v -args -evidence-dir=<隔离目录>` | 提 PR 前；真实 query 返回消息/变化、查询不增加 revision/native turn；原反馈保持 | acceptance EOV-A05，链接 Runner 专属证据 |

人工窗口 EOV-M01：两仓合并构建部署后，用户在真实执行详情确认信息足够判断活动，检查长消息与多候选切换体验；不阻塞代码交付。任务 105 的原生 GitHub 认证修复/重试不在本轮范围。

不选择 L2/L3 live 或 full：本次是只读展示与观察摘要；隔离数据库、公开 HTTP、前端时钟和 Runner 受控实际 EXE 足以覆盖自动项。不创建真实 PR 业务任务，不修改用户安装目录或认证。

跨仓依赖：[SteerRunner PR #6](https://github.com/erranliu/SteerRunner/pull/6)，实现提交 `83d902d`。先合并 Runner，再合并本仓并用两仓新源码构建三 EXE；安装配置继续按新 EXE SHA 固定，不使用旧 Runner 回退。PR 合并与人工体验验收尚未完成，不以已提 PR 声称已上线。
