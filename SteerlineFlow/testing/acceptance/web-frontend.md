# 本机 Web 前端验收

专属普通开发任务：[Spec](../../spec/web-frontend.spec.md)、[Plan](../../plans/web-frontend.plan.md)。2026-10-14 用户直接授权实现，不分配 roadmap 编号。要求与命令映射归 Spec/Plan，本页维护实际结果。

| ID | 行为 / 证据要求 | 状态 |
| --- | --- | --- |
| WEB-A1 | 独立 npm 工程、TS/生产构建、真实静态嵌入及普通 Go 构建 | 通过 |
| WEB-A2 | 认证/一次性票据/Origin/CSRF/Host/固定端口/私有文件/静态边界；native 保持 | 通过 |
| WEB-A3 | 持久化先行、原 ID 对账/unknown/原信封补交、刷新/跨标签/实例隔离/终态和异常回执 | 通过 |
| WEB-A4 | 原 DTO 与动作映射、任务/DAG/执行/回答/Activity/final/PR/能力和离线 | 通过，见覆盖边界 |
| WEB-A5 | IME/Enter、草稿/晚到响应/分页隔离、安全正文、深链接/窄屏 | 通过 |
| WEB-A6 | 新独立 SteerlineTest simulate 包的真实浏览器闭环和进程清理 | 通过 |
| WEB-M1 | 用户观察桌面输入法、布局和原生查看窗口；自动验证后参与，窗口待安排 | 待人工，可后置 |

## 自动证据（2026-10-14）

- `npm --prefix web ci/run build` 已通过；普通及 `-tags webui` Go 构建通过。实际嵌入由唯一调试生成器产出的包验证，普通构建无 Node/资源前置。
- 实现阶段前端检查 `npm --prefix web test`：5 个文件、17 项通过（18:19）。包含原请求持久化与锁、unknown/权威缺失、终态和错配回执、完整 UTF-8/SHA 分段正文、候选参与资格、IME/Enter/文本草稿与安全正文，以及离线查询暂停但保留成功缓存时禁写的回归。
- HTTP/夹具定向命令：`go run ./cmd/test-feedback -- ./web/server ./internal/product/httpapi ./internal/product/httpclient ./tests/e2e/internal/controlled -count=1`，通过；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T102021.128609800Z/summary.json`。覆盖 Web 认证和非法来源、私有 discovery/固定端口/静态边界、原信封转接、native 原契约、独立预算和取消、原 thread 只读分页。
- 决策接线定向 `go run ./cmd/test-feedback -- ./tests/e2e/internal/controlled ./internal/app -run 'Test(WebHistory|Decision|Answer|NativeAsync|AsyncDecision)' -count=1`，通过；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T094330.838493800Z/summary.json`。使用同一已绑定 store，保留原决策/attempt/recovery/版本事务资格。
- 最终固定 L1：通过，execution 264.6s；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T102021.129147100Z/summary.json`。已登记 `quick-owner-web`、`quick-owner-http-budget`。
- 文档链接、Task ID 静态核对及 `git diff --check` 均通过；这些静态检查不代表外部业务状态。

## 真实浏览器与覆盖边界

实现阶段包：`C:/SteerlineDebug/dev-20261014-182018-1fdb7265-dirty`，固定 Web `127.0.0.1:18744`，以 `start.bat --simulate` 冻结方案。源码基线及二进制/模板/generator 指纹归包 `manifest.json`；当时工作区含未提交修复，不能仅按基线 commit 重建。额外源码差异和 SHA 归 `evidence/verification/source-state.json` 与 `source-changes.patch`。产品二进制 SHA256：`fee298ff3e156b8db4b6f852683f8f59adc96e24cf4b21776177b6e8cfafde61`。

完整 `web.smoke` 在原生 Edge headless 中通过，1 项、7.8 分钟（无重试、未启用 diagnostic continuation）。日志 `evidence/browser-run.log`、结果 `evidence/browser/.last-run.json`、DAG/执行页宽窄截图 `evidence/browser/`；另人工看图核对实际加载后的 DAG 和交付后完整 final，补图为 `evidence/dag-loaded-*.png`、`final-*.png`。1440px/420px 都没有水平溢出，控制台 pageerror 为 0。

实际覆盖一次性授权/移除票据、首次启动丢回执后按原 ID 对账且仅一次 POST、完整 final、中文文本草稿刷新、原 session/execution/attempt 的第四轮原 Turn 核验、合并后 Task 1 交付与 Task 2 依赖解锁、离线禁写/重连、关闭第二标签后服务仍 ready。闭环后再查询权威，事实记录为 `evidence/closure-facts.json`：派发恢复/启动为 `applied`，回答为 `accepted`，保留原协议状态；Task 1 `closed`、Task 2 `open` 且 dependency_ready=true，不将回答受理改记为 applied。

以本包 native `stop-service` 收尾，回执 applied、process_exited=true、locks_released=true；原始记录 `evidence/stop-service.json`。启动 wrapper 正常结束，`evidence/cleanup.json` 确认本包拥有的进程为 0，18744 的监听为 0。包和证据保留，未重置或清理安装环境。

前一包 `dev-20261014-180531-1fdb7265-dirty` 已实际证明：一次性授权、真实任务/候选、丢失 start 回执后原 ID 对账、正式 final 读取、中文草稿刷新恢复、原 session/execution 的第四轮 publish feedback、模拟 PR 合并后交付与后继依赖解锁，以及 1440px/420px 布局。该轮在离线检查失败，不能记作整体通过；定位为 Query 离线暂停后缓存仍被当作在线，已补明确网络状态绑定和上述第 17 项回归。此前冷启动等待上限、模拟重复建 PR、原生写响应上限的问题也已修复；保留旧包失败日志，不用延长等待掩盖业务失败。

首次启动/派发恢复/回答为真实浏览器业务提交；并行候选、停止参与和原生查看的映射由原协议定向检查及本次代码核对覆盖，未声称进行了 live 多候选或实际原生窗口验收。业务 worker/GitHub 使用受控模拟外部协议，Go/Web/Edge、持久请求及产品权威为真实实现；未执行真实模型或真实 GitHub PR 操作，未使用安装环境。真实桌面 IME 与原生窗口归 WEB-M1，仍待人工体验。

## 已交付任务的首次启动提示回归（2026-10-15）

WEB-A4：用户报告 `127.0.0.1:18743` 的任务 101 已交付却显示“首次启动条件未满足”。仅通过正式 `task show` 只读查询确认现场为 `status=closed`、`dependency_ready=false`、`startup_blockers=[task_not_open]`，有 PR #250 的交付记录（`merged_at=2026-10-15T02:39:40Z`）；未修改或重启安装环境，也未将其作为调试夹具。

首次启动资格不适用于执行中或终态任务。任务图现只对 `open` 节点展示首次启动阻塞提示，保留原始任务状态和后端资格事实。新增 6 项隔离前端用例，覆盖与任务 101 相同的已交付快照、执行中、已放弃，以及 open 任务的依赖阻塞、派发阻塞和就绪状态。修复前 3 个非 open 用例失败；修复后 `npm --prefix web test` 为 5 个文件、32 项通过，`npm --prefix web run build`（TypeScript 与 Vite 生产构建）通过。未执行 Full、live 或浏览器交付闭环；本次修改仅为图节点提示投影，安装版仍需后续更新才能显示修复。

## PR 前质量审计与主干整合（2026-10-14）

用户授权审计、直接修复并提 PR。整合主干 `fc4f1efd` 后，审计修复调用方修改原信封、重复存储观察堆积对账、离线/关闭/候选版本变化后的晚到资格提交，以及取消缺失查询仍补交的问题；final 分页新增游标/身份/范围保护，结束候选显示历史问题。

- WEB-A3/A5：前端 5 个文件、26 项通过（19:58）；新增 9 项延迟 Promise/持久化/取消反例。TS 与生产构建通过；生产依赖 `npm --prefix web audit --omit=dev --audit-level=high` 为 0 漏洞。
- WEB-A2/A4：Web/HTTP/app/helper 定向检查通过，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T120043.412359800Z/summary.json`。主干整合后的固定 L1 通过，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T120043.430892900Z/summary.json`。普通与带 webui tag 的构建通过。
- WEB-A4/A6：整合主干后的新包 `dev-20261014-200042-4b2878dc` 在第四轮检查失败，日志与 trace 保留；权威证明 answer_decision 为 accepted，随后 PR 轮询替换了 answer_saved。新增 `TestCandidateOpenPRPollPreservesSavedFeedback` 先失败（报告 `.../20261014T122254.443169300Z/summary.json`），修正原可选问题生成器保留 answer_saved。5 项 PR 轮询/反馈阶段定向测试通过，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T122424.288990500Z/summary.json`；未以延长等待掩盖该问题。失败包已 stop-service 正常结束，本包进程/18745 监听为 0。

审计后最终包为 `C:/SteerlineDebug/dev-20261014-202454-3cec1823`，源码 `3cec1823`、source_dirty=false，固定 Web `127.0.0.1:18746`，由唯一生成器新建后 `start.bat --simulate` 冻结。产品 SHA256：`44869fe6204096d2c510ed93c0ba0888d377813d170b6a48c4a7a5ee5ff444ea`。

完整 Edge `web.smoke` 通过：1 项、5.0 分钟、无重试、无 diagnostic continuation。实际覆盖本页上述原 ID 丢回执对账、中文草稿刷新、完整 final、原 session/execution/attempt 第四轮、交付/依赖解锁和离线重连；实际查看 DAG/执行宽窄截图，420px 无水平溢出，pageerror=0。证据为本包 `evidence/browser-run.log`、`evidence/browser/`，权威收尾事实为 `evidence/audit-closure-facts.json`：4 次 dispatch、原 Run delivered/completed、Task 1 closed、Task 2 open/dependency_ready；resume/start applied，answer accepted，状态原样登记。

本包 stop-service 为 applied、process_exited=true、locks_released=true，wrapper 正常退出；记录 `evidence/audit-stop-service.json`、`evidence/audit-cleanup.json`，本包拥有的进程/18746 监听均为 0。其后仅补充验收记录及驱动超时说明，无实现变更。WEB-A1–A6 保持通过，WEB-M1 仍待人工；未扩展为 Full/live/远端验收。

## 并行候选方案选择修复（2026-10-15）

用户直接授权取消相同方案的参与限制。Web 新增候选先打开本次执行方案选择面板，确认时校验所选方案可用；默认方案不可用也可切换其他方案。服务及 SQLite 现有准入无方案去重限制，本次仅修改 Web 校验，保留五个参与候选上限和任务/派发资格。

- WEB-A4 定向组件回归：`npm --prefix web test -- src/features/tasks/task-detail.test.tsx`，5 项通过，覆盖与已有候选相同方案、不同方案的提交与版本参数、不可用默认方案切换、五候选上限和首次启动。
- Web 前端回归：`npm --prefix web test`，6 个文件、37 项通过；`npm --prefix web run build` 的 TypeScript 检查及生产构建通过。
- 本次未运行真实浏览器、多候选 worker 或真实 PR 验收，既有浏览器证据仅代表其原有版本与覆盖范围。
