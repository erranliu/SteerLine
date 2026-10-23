# 恢复 Runner 原会话查看

普通缺陷修复，关联[专属验收](../testing/acceptance/native-session-viewer.md)。拆分后原生 owner 已迁到 Runner，但产品仍保留未接线的按钮。本次同时补 Runner 公开 CLI 和 SteerLine 消费端，不读取 Runner 私有数据，不恢复旧执行 owner。

## 实施

Runner 提供 `open-native-session`，只接受公开 Run/Attempt/Session/Generation；在原 worker gate 内校验当前身份，经适配器核对原进程、冻结 binding 和 `thread/read` 工作目录，再打开无 prompt 的 `codex resume --remote` 窗口。只支持明确声明能力的 Windows Codex；不启动服务、不恢复或提交业务输入。查看器响应丢失保持未知，不自动重开。

SteerLine 从固定 EXE 的已核验 capability 决定服务能力；通过候选自己的 state-dir、输入摘要及分支核对公开 Run，再把原会话目标交给 CLI。沿用 HTTP 原请求落盘和不可重放规则。前端区分离线、能力缺失和会话身份缺失，并核对对应 adapter 的能力。

生命周期节点、异常及验证规则沿用 [worker-lifecycle](../../docs/product-design/worker-lifecycle.md)。本次只补只读定位与查看窗口，不改变 W01–W18、异常分类、重试或恢复授权。

## 测试计划

| 验收项 | 级别/场景与命令 | 环境、时点 | 通过条件、证据 |
| --- | --- | --- | --- |
| NSV-A01 | Runner unit：`go test ./internal/application ./internal/cli ./internal/execution/adapters/codex ./internal/execution/process -run 'Test(NativeViewer\|NativeConsole\|SRCLIContract)' -count=1 -timeout=2m` | 既有隔离 fixture、loopback WebSocket、Windows helper；接线后 | 原身份可打开；过期身份、进程/工作目录变化零打开；只 handshake/thread-read、零业务输入及状态修改；Runner `.evidence/native-session-viewer/runner-tests.log` |
| NSV-A02 | caller/HTTP：`go test ./internal/runnerclient ./internal/product/httpapi ./internal/storage/sqlite ./internal/app -run 'Test(NativeSessionViewerCLIOriginalTarget\|RunnerActorUsesHostScheduler\|ClientHTTPRoutes\|HTTPNative\|UnifiedStartup)' -count=1 -timeout=3m` | 隔离子进程/SQLite/HTTP fixture；产品接线后 | CLI 精确身份，响应丢失不可重放，actor 能力跟随 Runner；产品借用 store/启动保持通过；`.evidence/native-session-viewer/caller-tests.log` |
| NSV-A03 | Web unit：`npm test -- --run src/features/runs/native-session.test.ts`；`npm run build` | web 目录；最终实现后 | 可用原身份启用，离线/缺能力/缺身份分别禁用并正确解释；`.evidence/native-session-viewer/web-tests.log`、`web-build.log` |
| NSV-A04 | 新建独立包：`scripts/new-debug-release.ps1 -SteerRunnerExe <本次 Runner EXE>`；包内 `debug-helper debug run --simulate --no-open`；web 运行 `npx playwright test tests/native-session.spec.ts` | `C:/SteerlineDebug` 的独立 SteerlineTest；`STEERLINE_WEB_CONFIG` 指向本包配置；定向通过后 | 实际产品 snapshot 声明查看能力，按钮提交原候选，重放原 HTTP 请求只调用一次公开 CLI；simulate caller fixture 记录开窗回执，不冒充实际 Codex 窗口或真实模型验收；原生读取/窗口由 A01 覆盖；专属 acceptance 登记现场与结果 |

不选择 full/L2/L3：本次不改执行流程或 PR 交付，最小自动验证覆盖公开调用、原生读取、窗口及按钮条件。真实模型运行和 Orca 不属于本次验证；人工体验可后置，不阻塞以上自动项。

NSV-A04 的统一独立测试入口：`scripts/test-web-smoke.ps1 -SteerRunnerExe <本次 Runner EXE> -WebAddress 127.0.0.1:<空闲端口> -BrowserScenario native-session.spec.ts`。入口先用生成器新建包，再从既有 WMI 独立测试上下文启动；不从受 kill-on-close Job 限制的交互终端直接托管服务。脚本沿用原生停止与精确自有进程/监听清理核验，失败保留原场景和日志。
