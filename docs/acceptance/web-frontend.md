# 本机 Web 前端验收

专属普通开发任务：[Spec](../spec/web-frontend.spec.md)、[Plan](../spec/web-frontend.plan.md)。2026-10-14 用户直接授权实现，不分配 roadmap 编号。要求与命令映射归 Spec/Plan，本页维护实际结果。

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
- 最终前端检查 `npm --prefix web test`：5 个文件、17 项通过（18:19）。包含原请求持久化与锁、unknown/权威缺失、终态和错配回执、完整 UTF-8/SHA 分段正文、候选参与资格、IME/Enter/文本草稿与安全正文，以及离线查询暂停但保留成功缓存时禁写的回归。
- HTTP/夹具定向命令：`go run ./cmd/test-feedback -- ./web/server ./internal/product/httpapi ./internal/product/httpclient ./tests/e2e/internal/controlled -count=1`，通过；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T102021.128609800Z/summary.json`。覆盖 Web 认证和非法来源、私有 discovery/固定端口/静态边界、原信封转接、native 原契约、独立预算和取消、原 thread 只读分页。
- 决策接线定向 `go run ./cmd/test-feedback -- ./tests/e2e/internal/controlled ./internal/app -run 'Test(WebHistory|Decision|Answer|NativeAsync|AsyncDecision)' -count=1`，通过；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T094330.838493800Z/summary.json`。使用同一已绑定 store，保留原决策/attempt/recovery/版本事务资格。
- 最终固定 L1：通过，execution 264.6s；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T102021.129147100Z/summary.json`。已登记 `quick-owner-web`、`quick-owner-http-budget`。
- 文档链接、Task ID 静态核对及 `git diff --check` 均通过；这些静态检查不代表外部业务状态。

## 真实浏览器与覆盖边界

最终包：`C:/SteerlineDebug/dev-20261014-182018-1fdb7265-dirty`，固定 Web `127.0.0.1:18744`，以 `start.bat --simulate` 冻结方案。源码基线及二进制/模板/generator 指纹归包 `manifest.json`；当前工作区含本次未提交修复，不能仅按基线 commit 重建。额外源码差异和 SHA 归 `evidence/verification/source-state.json` 与 `source-changes.patch`。产品二进制 SHA256：`fee298ff3e156b8db4b6f852683f8f59adc96e24cf4b21776177b6e8cfafde61`。

完整 `web.smoke` 在原生 Edge headless 中通过，1 项、7.8 分钟（无重试、未启用 diagnostic continuation）。日志 `evidence/browser-run.log`、结果 `evidence/browser/.last-run.json`、DAG/执行页宽窄截图 `evidence/browser/`；另人工看图核对实际加载后的 DAG 和交付后完整 final，补图为 `evidence/dag-loaded-*.png`、`final-*.png`。1440px/420px 都没有水平溢出，控制台 pageerror 为 0。

实际覆盖一次性授权/移除票据、首次启动丢回执后按原 ID 对账且仅一次 POST、完整 final、中文文本草稿刷新、原 session/execution/attempt 的第四轮原 Turn 核验、合并后 Task 1 交付与 Task 2 依赖解锁、离线禁写/重连、关闭第二标签后服务仍 ready。闭环后再查询权威，事实记录为 `evidence/closure-facts.json`：派发恢复/启动为 `applied`，回答为 `accepted`，保留原协议状态；Task 1 `closed`、Task 2 `open` 且 dependency_ready=true，不将回答受理改记为 applied。

以本包 native `stop-service` 收尾，回执 applied、process_exited=true、locks_released=true；原始记录 `evidence/stop-service.json`。启动 wrapper 正常结束，`evidence/cleanup.json` 确认本包拥有的进程为 0，18744 的监听为 0。包和证据保留，未重置或清理安装环境。

前一包 `dev-20261014-180531-1fdb7265-dirty` 已实际证明：一次性授权、真实任务/候选、丢失 start 回执后原 ID 对账、正式 final 读取、中文草稿刷新恢复、原 session/execution 的第四轮 publish feedback、模拟 PR 合并后交付与后继依赖解锁，以及 1440px/420px 布局。该轮在离线检查失败，不能记作整体通过；定位为 Query 离线暂停后缓存仍被当作在线，已补明确网络状态绑定和上述第 17 项回归。此前冷启动等待上限、模拟重复建 PR、原生写响应上限的问题也已修复；保留旧包失败日志，不用延长等待掩盖业务失败。

首次启动/派发恢复/回答为真实浏览器业务提交；并行候选、停止参与和原生查看的映射由原协议定向检查及本次代码核对覆盖，未声称进行了 live 多候选或实际原生窗口验收。业务 worker/GitHub 使用受控模拟外部协议，Go/Web/Edge、持久请求及产品权威为真实实现；未执行真实模型或真实 GitHub PR 操作，未使用安装环境。真实桌面 IME 与原生窗口归 WEB-M1，仍待人工体验。
