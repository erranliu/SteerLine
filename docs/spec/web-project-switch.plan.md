# Plan：独立 Web 程序与已有项目切换

范围归 [Spec](web-project-switch.spec.md)，结果归[专属验收](../acceptance/web-project-switch.md)。用户直接授权普通开发规划，不纳入 roadmap；本轮仅写文档，实施尚未开始。代码基线 `b657baf39d82ff33e47c7175799421c340748a8d`；已读取 `docs/spec/SpecGuide.md` 全文，其最后修改 commit 为 `d633d1110b4a54c555aa9f67dedb779a7cf9d807`，本次文件 SHA256 为 `8b340e17b0f402e924b3ddcb20e91e902bba0421b425921bd7cc62d946d45dd3`。

## 整体整合与依赖

本任务先交付，后续[项目添加 Plan](project-onboarding.plan.md)只消费它的 Go 应用 host、项目目录及应用协议，不重建网关。添加提示词、结构清单、核验/登记、初始化闸口均不进入本批。两个任务各有专属验收，不共用一张结果表。

| 现有位置 | 本次处理 |
| --- | --- |
| `web/server`、`cmd/steerline/web.go` | 保留认证/静态/开发代理，改接应用客户端；拆出独立 CLI，删除原按项目 CLI 与 discovery |
| `internal/app/webui.go`、`execution_service_wiring.go`、`project_config_input.go` | 删除项目宿主启动/关闭 Web 及 WebAddress 字段，保留原 native endpoint 生命周期 |
| `internal/consoleapp/console.go`、`app/config.go` | 提取现有只读连接配置/Git 身份能力为无 UI 依赖的解析器；Go 应用目录使用，TUI 仅保持已有功能，不让后端依赖 consoleapp |
| `internal/product/httpclient`、`httpcontract`、`httpfs` | 复用原项目连接、请求/回执及私有文件机制；应用协议用独立 DTO，原项目协议不另写一遍 |
| `web/src/app`、`shared/api`、`features` | 增加全局会话/项目目录与按项目 Workspace，保留页面内容及已有原信封/输入/final 防护 |
| `build.ps1`、launchers、`scripts/new-debug-release.ps1`、测试驱动 | 双产物及独立生命周期，所有实际调试仍走唯一生成器 |

业务目录逻辑位于 Go 应用层。建议应用协议契约、handler、纯客户端分别放 `internal/product/applicationcontract`、`applicationhttp`、`applicationclient`，应用组合放 `internal/app`；这些是必要的协议与所有者边界，不引入通用服务注册框架。应用服务只持有到项目的 HTTP 连接，不持有项目/worker 的终止能力。

## 实施步骤

1. 固定应用级 identity/catalog/按项目转接 DTO 和错误码；实现无项目也能启动的 `serve-app`、`stop-app`、应用私有 discovery/ACL、单实例锁、正常退出与真实进程取证。应用级正常停止能力供 CLI/测试驱动使用，Web 不代理该停止路由，不添加浏览器停项目入口。
2. 提取无 UI/无存储写入的现有项目连接解析，加入逐项目限时与诊断、重复 common-dir 冲突、刷新及连接租约/替换。不得调用 `OpenNativeTaskProject`、启动检查或任务命令来发现项目。为并发在途调用关闭和实例变化补最小测试；本批新增后端测试统一使用 TestWebSwitch 前缀。
3. 用原 httpclient 受控转接项目路由；在转接前检查目录、路径、项目/实例和凭证归属，保持原错误及回执。应用离线、项目离线、协议冲突与 command_not_found 分开。
4. 创建 `cmd/steerline-web` 的 serve/open/stop/install，改造原 server。浏览器 status 只表达 Web/home/应用连接；项目 identity 从项目路由取得，新项目路径深链接提供 SPA 静态回退。保留票据、cookie、CSRF、CSP 与有界 native 调用，不将任何原生令牌发往浏览器；Web 原生 stop 与浏览器会话路由隔离。
5. 增加项目切换器、项目路径路由及零项目页，调整 App/Workspace 的生命周期和 query key。回执/草稿按 home/项目/实例隔离；恢复原操作不依赖当前选择，禁止晚到资格回调向新项目发命令。补两标签、同号 Task、重名、离线、final 和切换竞态测试。
6. 删除项目 `web_address` 及宿主 Web 接线/旧 CLI，更新示例、README 和独立启动器；默认业务 build 不装 npm。`build.ps1 -Web` 用既有源目录路径守卫构建业务 exe 与带资源的 Web exe；各自 install 使用同 home 并拒绝覆盖正在运行的目标。安装/升级不默认触及 `C:\SteerlineRuntime`。
7. 扩展唯一生成器 `-WebProjects 1|2`（默认 1；双项目只允许在 Web 包模式），两份独立 SteerlineTest clone、provider 状态、应用配置、双 exe 与启动器均由维护的模板/生成器产生。沿用原 simulate 冻结规则，不另写测试安装器或拼凑运行目录。
8. 新增 `web/tests/project-switch.spec.ts`，沿用 Edge/单 worker/不重试/截图 trace。扩展驱动为两个项目服务、应用及 Web 的精确启动、重启/退出与取证；复用 controlled RecoveryFixture.HoldStep/release 将 A 保持在原生活动轮次，分别重启 Web 和应用入口，核对原项目服务 PID/instance、Execution/Attempt/session/turn 不变，再释放原轮次。随后在原轮次结束后正常重启项目服务，证明 UI 按新 native 实例重连且旧信封不改绑，不触发 worker 自动恢复。不能仅凭“PID 存在”宣布 worker 未受影响。
9. 调整原 web.smoke 到新入口，保留原反馈/final/交付闭环；登记新小型 owner 测试进入 L1，运行下表必要检查，更新验收及当前架构/运行指南。后续添加功能仍无入口，无假按钮或预初始化。

## 测试计划

下列新增包/测试/脚本参数目前不存在，须按上述步骤补齐；执行时 runner 必须匹配到实际场景且非零测试，不能用零匹配成功充数。证据写 runner 新临时目录或本次新包 evidence，在[专属验收](../acceptance/web-project-switch.md)逐项登记来源、版本、结论与清理。Plan 不维护实时结果。

| 验收项 | 级别 / 组 / 场景 | 环境与命令 | 时点 / 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [WPS-A1](../acceptance/web-project-switch.md) | 定向 `web-switch.build` | Windows/Go/Node；`./build.ps1`、`./build.ps1 -Web`；干净隔离环境检查无 npm 时普通业务构建；临时 home 分别执行双 exe install/launchers | 包装完成后；产物、资源归属、固定地址冲突、在运行时拒绝覆盖、启动器及普通构建通过 | 验收 WPS-A1；构建/安装与二进制 hash |
| [WPS-A2、WPS-A3、WPS-A5](../acceptance/web-project-switch.md) | Go 定向 `web-switch.backend` | 隔离 fixture；`go run ./cmd/test-feedback -- ./internal/app ./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -run '^TestWebSwitch' -count=1` | 应用/认证/转接完成后；零项目、坏/慢/冲突目录不建库、鉴权拒绝、原回执/错实例及租约替换全命中 | 对应 ID；runner summary 与场景日志 |
| [WPS-A4、WPS-A3](../acceptance/web-project-switch.md) | Vitest `web-switch.client` | 隔离浏览器存储；`npm --prefix web test` | UI 完成后；同号任务/重名/两标签/实例、草稿/final/延迟资格与 unknown 原信封反例通过，保留原 26 项适用检查 | WPS-A3、WPS-A4；Vitest 日志 |
| [WPS-A1、WPS-A5、WPS-A6](../acceptance/web-project-switch.md) | 真实浏览器定向 `web.project-switch` | `./scripts/new-debug-release.ps1 -WebAddress 127.0.0.1:<空闲固定端口> -WebProjects 2`，新包 `start.bat --simulate`；环境变量绑定该包后 `npm --prefix web run test:e2e -- project-switch.spec.ts` | 全接线后；同源两项目切换、一次原生写及丢回执原 ID、活动轮次中独立重启、重连与原轮次完成均通过；全部本包进程退出且监听为 0 | WPS-A1、WPS-A5、WPS-A6；截图/trace、PID/实例/原请求/轮次事实、退出/清理 JSON |
| [WPS-A3、WPS-A6](../acceptance/web-project-switch.md) | 既有接线回归 `web.smoke` | 另一个新 Web simulate 包，`npm --prefix web run test:e2e -- smoke.spec.ts`；不得复用前一场景任务状态 | 迁移原驱动后；原反馈第四轮、完整 final、交付/依赖、离线及清理通过 | WPS-A3、WPS-A6；原 smoke 日志和 closure/cleanup |
| [WPS-A1、WPS-A2、WPS-A3、WPS-A4、WPS-A5、WPS-A6](../acceptance/web-project-switch.md) | 最小回归 | `go run ./cmd/test-feedback -level=L1`；`go run ./cmd/test-feedback -- ./internal/product/httpapi ./internal/product/httpclient ./tests/e2e/internal/controlled ./tests/e2e/internal/driver -count=1`；`npm --prefix web run build` | 最终候选；旧 native/TUI 边界、辅助协议/驱动及构建通过，新 owner 小测试已登记 | 回归汇总，不能代替上述产品场景 |
| 文档 | 静态 | `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check` | 文档收尾；无错误 | 验收记录的静态摘要 |

不选择 Full、全部 L2/L3、真实模型/真实 PR 或远端矩阵：本批不改 worker 行为，隔离后端测试与真实本机 Edge/simulate 足以核验连接/隔离/进程边界。WPS-M1 在自动通过后由用户体验，窗口待安排，只阻塞该体验签收。实现及必要自动验收完成后再报告交付；是否提 PR/安装另按实际用户授权处理。
