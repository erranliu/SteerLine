# 独立 Web 程序与项目切换验收

专属普通开发任务：[Spec](../spec/web-project-switch.spec.md) · [Plan](../spec/web-project-switch.plan.md)。2026-10-15 用户直接授权实施，已完成实现及 WPS-A1～WPS-A6 自动验收；与后续项目添加分开登记，无 roadmap Task ID。

| ID | 最小检查与通过条件 | 当前状态 / 证据 |
| --- | --- | --- |
| WPS-A1 | 双 exe 构建、资源只进 Web、无 Node 业务构建、临时 home 安装及运行目标拒绝覆盖；独立启动器、单端口及热更新接线 | 通过；命令、版本、模拟范围及证据见下方执行记录 |
| WPS-A2 | 零项目/离线/错误/慢配置目录可用；同仓冲突与跨 home 隔离；核对枚举前后项目数据库/状态无新建 | 通过；命令、版本、模拟范围及证据见下方执行记录 |
| WPS-A3 | 非法来源/身份/路由/CSRF 拒绝，原生令牌不出浏览器；原信封/回执及原 ID 缺失/unknown/错误语义 | 通过；命令、版本、模拟范围及证据见下方执行记录 |
| WPS-A4 | URL 当前项目、同号任务/同名项目、标签页选择独立、草稿/final/迟到响应及离线禁写 | 通过；命令、版本、模拟范围及证据见下方执行记录 |
| WPS-A5 | A 原生活动轮次中独立重启 Web 和应用入口；项目 PID/instance 与执行/会话身份保持，原轮次释放后完成；项目服务新实例重连不重写旧信封 | 通过；命令、版本、模拟范围及证据见下方执行记录 |
| WPS-A6 | 新双项目包真实 Edge 场景及另一新包原 smoke 闭环；原生业务写/丢回执对账、截图/trace、退出与本包进程/监听 0 | 通过；命令、版本、模拟范围及证据见下方执行记录 |
| WPS-M1 | 用户体验当前项目布局、切换和独立重启入口；自动通过后参与，窗口待安排，可后置 | 待人工；仅阻塞体验签收 |

## 执行与证据登记

Plan 的测试表给出命令、能力补齐、环境和执行时点。`web.project-switch` 的双 clone/provider、受控活动轮次、双 exe 重启/进程取证和清理驱动均须先实现，零匹配不算执行；不以调用 fake client 或只看 PID 替代原生活动轮次检查。

执行后每项记录源码 commit/dirty、包目录、场景及命令、开始/结束、实际结果、原始报告路径和限制。失败与未验不写成通过；Go/Node/Edge 为真实实现，simulate 外部协议范围单列。实际产品调试只用生成器新包和 SteerlineTest，AI 显式 `--simulate`，不使用安装目录。

浏览器场景结束后逐个正常停止本包 Web、应用和两个项目服务，保存其退出事实，核对本包 owned 进程及监听为 0。若为故障场景受控结束某进程，记录其精确身份和范围。前端关闭不自动停业务服务；测试收尾由独立驱动显式完成，不能混淆两者。

静态检查不替代产品场景；未运行 Full、真实模型/PR 或远端矩阵。

## 2026-10-15 实施与自动验收记录

代码基线 `f507b7183e95acd80c06de6b2b380e6e10c63674`，本次修改为 dirty 工作树；每个新包的 manifest 保存业务/Web/helper SHA-256。未触及 `C:\SteerlineRuntime`，没有安装现场更新、真实模型调用或 GitHub PR。

- WPS-A1：移除 Node/npm 的 PATH 后运行 `./build.ps1` 成功；`./build.ps1 -Web` 正式双产物构建通过，`go list -deps ./cmd/steerline` 确认业务生产依赖不含 `web/server`。新包 `C:\SteerlineDebug\dev-20261015-105910-f507b718-dirty` 的 `evidence/web-switch-build.json` 记录双 exe 安装、运行目标拒绝替换、启动器及独立正常停止通过。Vite 静态转接/API 留在 Go/同 home 租约冲突由 `TestWebSwitchDevelopmentProxyAndLease` 验证。
- WPS-A2/A3：零项目、只读不建库、坏/慢/同仓冲突、删改配置撤销映射、跨 home、原生来源/路由、完整原信封及超过 JS 精确范围数字不变、权威缺失与不可用区分、在途调用与新连接替换均有实际命中的定向测试。报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T020657.743399100Z/summary.json`；最终连接缓存/原生重启与显式 HoldStream 夹具定向报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T024420.793910400Z/summary.json`（28.927s）。
- WPS-A3/A4：Vitest 6 文件、34 项通过（最终 2026-10-15 11:31，4.86s；收尾日志见下），保留原 26 项并新增同名项目路由/同号目标清除、标签页独立、无效深链、home 持久隔离、排队命令取消、旧实例只读查证且禁补交、异步目录完成后恢复最近项目、原信封落盘期间切换时取消明确未发送的请求。浏览器截图检查后已修正切换器的深色背景/可读性，并显示当前名称、位置、连接状态及不可选配置诊断。
- 最小回归：`go run ./cmd/test-feedback -level=L1` 通过（源码含新 owner 用例，187.9s），报告 `.../20261015T021419.714947900Z/summary.json`。原 native HTTP/客户端/controlled/driver 全包定向回归通过，报告 `.../20261015T013826.954602000Z/summary.json`；新增 driver/controlled 变更后再次两包检查通过（26.076s/8.187s）。CLI 两包检查通过，业务 CLI 159.768s，Web CLI 构建通过。TUI 的只读连接/路径守卫已同步纳入精确依赖白名单，仍禁止 backend/SQLite 依赖。
- WPS-A5/A6：真实 Edge 调试发现并修复连接探测预算、已验证连接复用、测试夹具 HoldStep 断流与 WebFeedback 初次发布识别、浏览器首次选择顺序及调试启动器的进程所有权问题。失败样本保存在对应新包的 evidence；它们不记为整体通过。`dev-20261015-094907-*` 与 `dev-20261015-100132-*` 已记录重启前后原项目/执行/会话/轮次事实及正常清理 `processes=0/listeners=0`，释放后的完整闭环已在下方最终新包通过。

人工 WPS-M1 未执行；仅阻塞体验签收，不代表自动结果。

### 已完成的真实单项目回归

`./scripts/test-web-smoke.ps1` 在全新 simulate 包 `C:\SteerlineDebug\dev-20261015-104407-f507b718-dirty` 中通过真实 Edge `web.smoke`（1 项，6.7 分钟）。证据：`evidence/smoke.log`、`evidence/browser/` 的 DAG/桌面执行页/窄屏截图；`evidence/smoke-cleanup.json` 为 `browser_passed=true`、`passed=true`、`processes=0`、`listeners=0`、`stop_errors=[]`。覆盖原生业务提交、反馈第四轮、IME 草稿、完整 final、合并交付、依赖和离线禁写。外部模型、GitHub/PR 由本包 controlled provider 模拟，产品 Go/React/Edge 及本机 HTTP/进程真实运行。

双项目初次签收包 `dev-20261015-105910-f507b718-dirty` 因测试仅等待 60 秒就绪失败；随后两个项目均正常启动，收尾清理为 0/0，证据保留。已将该场景等待预算与单项目一致调整为 180 秒；不把该失败样本记为通过。

### 最终双项目真实浏览器签收

2026-10-15 11:19:20～11:28:30（Asia/Shanghai），全新包 `C:\SteerlineDebug\dev-20261015-111706-f507b718-dirty`，命令为设置该包的 `STEERLINE_WEB_PACKAGE` / `STEERLINE_WEB_EVIDENCE` 后运行 `npm --prefix web run test:e2e -- project-switch.spec.ts`。真实 Edge 单 worker、不重试，1 项通过（9.1 分钟）；`evidence/browser/.last-run.json` 为 `passed`。

- 双独立 clone/provider 在线，同源切换与两标签选择独立通过；同号 Task 未串到另一项目。丢失首次 start_task 浏览器回执后按原 ID 查证为 applied，业务提交计数始终为 1。
- 原轮次暂停于 implement.execute，分别正常停止并重启 Web、应用服务，原项目服务 PID/instance、worker 原生进程 PID/创建时间/可执行文件、Execution/Attempt/session/turn 均保持。释放原 turn 后沿同一 Execution/Attempt/session 完成三阶段发布，simulate PR 合并并产生 TaskDelivery。
- 随后正常重启 A 项目服务，B 的原实例继续可用；A 按新 native 实例重新连接，旧回执仍绑定旧实例。provider 保持 1 session、3 dispatches，没有因重连启动新 worker。停止 B 后可选择离线状态并禁写，A 的原回执仍可读取。
- 收尾正常停止本包 Web、应用与两个项目服务，`evidence/web-project-switch.json` 为 `submissions=1`、`cleanup.processes=0`、`cleanup.listeners=0`、`stop_errors=[]`。截图、完整 trace 位于 `evidence/browser/project-switch-web-project-d29a6-active-turn-survive-restart/`，过程日志为 `evidence/project-switch.log`；生成 manifest 固定三份 exe 的 SHA-256。

关键事实：原 request/run `a7befdd1817b453c856fd74ac48679df`，Execution `a8c203b12c060bfbdffc0a42eb192fe3`，Attempt `2877c01d3a6b937260d0ad219d3ee953`，session `e5bc1b30-9693-404d-9697-561bb741b4b0`，原 turn `27f66205-4ade-421e-a464-a11a163cdc09`；项目 A 原实例 `66d2b6d7ac242039fcd6fe8ce2cf4514`，重启后实例 `2e6468dd5bd52623f44b579df83e3ccf`。原始完整事实归上述 JSON/trace。

前一包 `dev-20261015-110416-f507b718-dirty` 已完成双重启、原轮次、合并和新实例检查，但最终离线文案精确匹配失败（实际页面为“项目离线，当前不可提交业务操作”）；已修正测试覆盖有/无缓存两种离线呈现，该包也清理为 0/0，失败记录保留。最终通过包不复用该包业务状态。

综合场景之后补充的“落盘期间切换取消”窗口由新增 Vitest 反例验证，所有 34 项及生产构建再验；最终源码构建/测试日志位于最终双项目包的 `evidence/final-build.log`、`evidence/final-vitest.log`，最终业务/Web 二进制 hash 见 `evidence/final-build.json`。该竞态未使用浏览器计时注入，不能把单元反例误记成 Edge 注入场景。

最终 `./build.ps1 -Web` 已验证覆盖已有业务/Web exe 的增量构建。第一次收尾暴露 PowerShell 向 .NET Replace 传空备份路径的问题，修正为与业务产物一致的受守卫临时备份替换，再次构建通过；失败原始日志保留为 `evidence/final-build-first-attempt.log`。

最终静态收尾：文档导航 418 文件、2663 本地链接无错误；97 份 Task ID 身份文件检查通过；`git diff --check` 通过。WPS-M1 仍待用户体验布局与日常独立启动/停止入口，可后置。

## 2026-10-15 提 PR 前质量审计

按用户要求复审实现、测试与交付接线；提交分支已从初始实现基线同步到主干 `411110d9`，保留 #318 的任务启动提示修复及 #319 的并行候选方案修复。下列问题均已修正，未新增旧协议/数据回退路径。

| 发现 | 修复及最小反例 | 验收归属 |
| --- | --- | --- |
| Git 父进程退出后，后代继承输出管道会使 Output 等待越过配置解析期限 | 设置 250ms WaitDelay；真实测试子进程派生持有管道的后代，解析须在 2 秒内返回，夹具通过释放文件正常退出 | WPS-A2 |
| 空 common-dir 输出可错误地退回仓库目录，异常大输出未限制内存 | 空输出拒绝；stdout 最多 64KiB，错误输出保留明确诊断；空/超量输出均不能生成项目身份 | WPS-A2、WPS-A3 |
| 目录只读解析拒绝原生服务已支持的当前 UTF-8 BOM 配置 | 与现有原生配置解码一致去除 UTF-8 BOM；有/无 BOM 均解析至真实 common-dir，且不创建 state | WPS-A2 |
| 标签页保存的项目被删除后，会遮挡仍有效的浏览器最近项目 | 按优先级选择第一个有效候选，保留异步目录完成后的恢复检查 | WPS-A4 |
| 偏好读写被浏览器拒绝时，异常会阻止项目页加载或切换 | 最近偏好作为尽力保存；存储抛 SecurityError/QuotaExceededError 时仍可按 URL 切换，原信封/草稿持久化的写入守卫不放宽 | WPS-A4 |

审计验证：

- `go run ./cmd/test-feedback -- ./internal/projectconnection ./internal/app ./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -run '^TestWebSwitch' -count=1` 通过（14.507s），报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T033758.703776000Z/summary.json`。
- 同步主干后 `npm --prefix web test`：7 文件、47 项通过（11:40:02，5.49s），`npm --prefix web run build` 通过；包含主干两项 Web 修复的原回归及本次偏好边界反例。
- `go run ./cmd/test-feedback -level=L1` 通过（173.2s），新增解析器 owner 已登记继承管道/异常输出检查，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T033952.819553200Z/summary.json`。随后补充 BOM 一致性反例并登记该 owner，定向重跑 `./internal/projectconnection ./internal/app -run '^TestWebSwitch' -count=1` 通过（12.678s），报告 `.../20261015T034438.596179400Z/summary.json`。
- 同步主干后 `./build.ps1 -Web` 双 exe 覆盖构建通过，日志/产物 hash 见 `C:/Users/lmas1/AppData/Local/Temp/steerline-web-switch-audit-20261015-114137/build.log`、`build.json`；BOM 增量后再运行普通 `./build.ps1` 通过。
- 上文真实 Edge 单/双项目综合结果属于实施轮次的明确包版本，本轮审计没有再次执行 Edge 综合场景，也不把定向反例声明成新的浏览器注入验收。WPS-M1 仍待人工体验。

## 2026-10-15 安装目录权限故障修复

用户报告 `service-SteerlineTest.bat` 突然拒绝访问。首次启动独立 Web 且 home 下没有 `web.json` 时，CLI 使用了会收紧父目录 ACL 的 `localservice.Write`。Windows 上 home 的当前用户 ACE 丢失 OI/CI 继承标志，原本依赖继承的 bat 随即变成空 DACL。配置创建时间与权限故障一致；隔离反例重现了同一 ACL 变化及 `Access is denied`，属于本次实现遗漏。

按用户本次安装维护请求，仅对 `C:\SteerlineRuntime` 根目录恢复当前用户 FullControl 的文件/目录继承；未使用递归重置、未给其他用户授权、未启动任何安装服务或任务。只读复核确认项目 service、service-window、start-app/start-web 等 bat 可读取，继承当前用户 FullControl。安装环境没有用作测试现场；未替换安装的 exe，也未修改其业务状态/配置。

源码修复将首次配置创建收口到 CLI 实际调用的 `EnsureAddress`，以私有临时文件原子发布 `web.json`，不修改 home 权限。原服务 discovery 的 `Write` 仍收紧其专用私有目录。新增 `TestWebSwitchHomeConfigPreservesPermissions` 并登记 L1 `quick-owner-web`：Windows 夹具模拟可继承 ACL，核对 home 安全描述符不变、已有 bat 可读、配置文件私有及已有自定义配置不被覆盖；其他平台夹具检查目录模式保持。对应 WPS-A1、WPS-A3。

本轮源码基线 `47fafc12721ceb8fa39ba1ab227872fd14513bcb`，分支 `codex/fix-web-config-home-acl` 的 dirty 工作树，实际验证如下：

- 修复前反例：`go run ./cmd/test-feedback -- ./web/server -run '^TestWebSwitchHomeConfigPreservesPermissions$' -count=1` 如预期失败；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T041821.491015500Z/summary.json`，原始 JSONL 同时记录 OI/CI 丢失和 bat 拒绝访问。
- 修复后：`go run ./cmd/test-feedback -- ./web/server ./internal/localservice ./cmd/steerline-web -count=1` 通过（10 项测试、两包无测试仅编译，7.456s），报告 `.../20261015T041854.127510400Z/summary.json`；涵盖 Web 认证、私有 discovery、租约与新增权限反例。
- 私有文件/应用端点及测试登记回归：`go run ./cmd/test-feedback -- ./internal/product/httpfs ./internal/product/applicationhttp ./cmd/test-feedback -count=1` 通过（93 项测试，106.486s），报告 `.../20261015T041947.882558000Z/summary.json`。
- `./build.ps1 -Web` 双 exe 构建通过；日志与 SHA-256 位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-home-acl-fix-20261015-121946/build.log`、`binaries.json`，产物仅在源码 `bin/`。安装里的 Web exe 尚未带入此源码修复；当前已有 `web.json`，该首次创建分支不会再次执行。
- 文档导航 418 文件、2664 本地链接无错误，`git diff --check` 通过。未重跑 Full、L2/L3 或 Edge 综合场景；本轮只改配置发布权限，使用真实 Windows ACL 的隔离反例与受影响包回归即可覆盖，未改 worker/业务流程。WPS-M1 状态不变。
