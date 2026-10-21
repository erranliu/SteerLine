# SteerLine Web

本机浏览器客户端，使用独立 Web 程序、Go 应用目录与原项目服务。[Spec](../docs/spec/web-project-switch.spec.md) · [Plan](../docs/spec/web-project-switch.plan.md) · [验收](../docs/acceptance/web-project-switch.md)。项目添加、远程 Web 和 SSE 尚未实现。

## 构建与启动

```powershell
./build.ps1 -Web
# 分别安装到明确选择的 home；运行中的目标会拒绝替换。
$env:STEERLINE_HOME = '<所选 home>'
./bin/steerline.exe install
./bin/steerline-web.exe install
./bin/steerline.exe launchers
```

普通 `./build.ps1` 只生成业务 exe，无 Node 前置。页面只嵌入 Web exe；运行两份 exe 都不依赖 Node。手工等效 Web 构建为 `npm --prefix web ci`、`npm --prefix web run build`、`go build -tags webui -o bin/steerline-web.exe ./cmd/steerline-web`。

安装目录双击 `start.bat`（等价 `steerline start`），短协调器依次启动并核验同 home 的共享后端和 Web，然后委托本 home 的 Web `open` 授权打开浏览器。重复启动复用实例；`--no-open` 适合自动化。`steerline status --json` 展示宿主、Web、项目和最近停机结果，`stop.bat` / `steerline stop` 正常停止两者并核验各项目收尾。

后端低层入口为 `steerline serve`，自动冻结并装载 `etc/*.json`，所有项目在同一 PID 内分别拥有数据库、actor、实例和锁。配置修改在下次宿主启动生效。Web 可通过 `steerline-web stop`、`serve`、`open` 独立重启，保持项目与 worker 活动轮次。

Web 地址保存在 `<home>/web.json`：`{"version":1,"address":"127.0.0.1:18742"}`；缺文件时采用该默认值并在启动时保存。稳定端口冲突明确失败。项目配置的 `web_address` 和旧 `steerline web -config` 已删除，旧配置需要显式移除该字段，不自动迁移。

当前项目使用 `/projects/{project_id}/dag`、`/runs`、`/settings` 深链接。切换器显示仓库位置、状态和诊断，可搜索；每标签页的选择独立。最近选择可共享，不能抢改已打开的标签页。配置目录来自 `<home>/etc/*.json`，坏配置保留诊断，同 common-dir 多配置拒绝路由；列表查询不初始化业务数据库；初始化属于持锁的项目装载流程。

## 认证与恢复

`open -print-url` 仅供受控浏览器自动化，原生 launcher 从当前用户私有 discovery 申请一次性 60 秒票据。票据在 fragment，页面兑换前移除；12 小时会话绑定 home/Web 实例，HttpOnly/SameSite Strict、精确 Origin/Host、CSRF 和生产 CSP 保留。应用与项目 Bearer 不进入浏览器。

Web 重启后再次 `open` 授权。应用服务重启后 Web 核对同 home 的新实例，再恢复只读连接。草稿与完整原请求存 IndexedDB，按 home/项目/实例及执行身份隔离；切换取消旧查询、资格检查和未发送回调。已发送命令只按原 ID 查询；原实例变化后仍可读取旧回执，但不把旧信封补交到新实例。离线只保留当前标签页已有缓存并禁写。

## 查询与缓存

服务读取统一使用 `src/shared/api/queries.ts` 的 TanStack Query options，按 home/项目/原生实例及输出身份隔离。连接核验后后台预取当前项目的执行方案和任务列表首屏；DAG/执行页共享任务列表，已有缓存先展示，后台刷新保留内容。任务列表和执行方案的在途读取可跨页面复用，项目/实例切换仍取消原 Workspace 的读取。

执行方案新鲜期 60 秒，不定时轮询；动态状态保留原刷新周期，仅可见的图/列表、活动/final 启动对应查询。操作后按资源族定向失效，手动刷新包含当前连接查询；缓存不替代服务器操作资格。后续 SSE 继续更新同一缓存，本批不实现 SSE 或查询持久化。[Spec](../docs/spec/web-query-optimization.spec.md) · [Plan](../docs/spec/web-query-optimization.plan.md) · [验收](../docs/acceptance/web-query-optimization.md)。

启动确认后立即进入执行页并选中提交卡片，允许继续新增至五个参与候选；编号和执行信息由服务器补齐。首次领取前的后续选择等待原领取核实，已发送请求沿原日志对账，迟到回执不改变选择。未发送的等待选择只在当前 Workspace 内存中保留，不跨页面重开自动派发。[Plan](../docs/plans/web-candidate-start.plan.md) · [验收](../docs/acceptance/web-candidate-start.md)。

## 独立调试与验证

```powershell
./scripts/new-debug-release.ps1 -WebAddress 127.0.0.1:18742 -WebProjects 2
& 'C:\SteerlineDebug\<新包>\start.bat' --simulate
& 'C:\SteerlineDebug\<同一包>\stop.bat'
```

Web 是唯一交互前端，原 TUI 已移除。默认 live，AI 显式 `--simulate`；首次方案冻结。生成器默认包含业务/Web 双 exe、helper、配置和启动器，两项目使用独立 clone/common-dir/provider，配置源在 `home/config-source`，etc 仅保存活动项目。包内 `reset-SteerlineTest.bat` 同时检查两项目、应用、Web 与启动器锁，再重置本包执行状态。不使用安装目录准备或运行调试。

热更新：先在 `web/` 运行 `npm run dev`，为 Vite 设置 `STEERLINE_WEB_DEV_PORT=18742`；启动本包 Web 前设置 `STEERLINE_WEB_DEV_ORIGIN=http://127.0.0.1:5173`。浏览器始终通过 Go 的固定 Origin，只有静态/Vite HMR 路由转到 Vite；认证和 API 留在 Go。

```powershell
npm --prefix web test
npm --prefix web run build
go run ./cmd/test-feedback -- ./internal/app ./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -run '^TestUnifiedStartup' -count=1
go run ./cmd/test-feedback -level=L1
```

真实 Edge 切换场景会启动和正常清理全新双项目包：设置 `STEERLINE_WEB_PACKAGE=<新包>`，运行 `npm --prefix web run test:e2e -- project-switch.spec.ts`。综合场景同时核验两份运行中的安装目标拒绝覆盖。推荐入口为 `./scripts/test-unified-startup.ps1 -Package <新包> -Simulate`；查询缓存另建新包并加 `-Scenario query-cache`。

`./scripts/test-web-smoke.ps1` 可自动生成另一新单项目包、启动、运行原 smoke 并正常清理。手动入口的原 `web.smoke` 使用另一个全新单项目 Web 包，先 `start.bat --simulate`，设置 `STEERLINE_WEB_CONFIG=<包>/home/etc/SteerlineTest.json` 后运行 `npm --prefix web run test:e2e -- smoke.spec.ts`；收尾显式正常停止 Web、应用和项目，核实本包进程/监听为 0。截图、trace 和原请求/轮次身份记录在 `STEERLINE_WEB_EVIDENCE` 指定的证据目录或默认临时目录。模拟范围仅限外部模型/PR 协议，Go、Git、SQLite 和 Edge 均为真实实现。

查询缓存浏览器场景 `query-cache.spec.ts` 使用另外生成的全新双项目包：设置 `STEERLINE_WEB_PACKAGE` 和 `STEERLINE_WEB_EVIDENCE` 后运行 `npm --prefix web run test:e2e -- query-cache.spec.ts`。测试负责 simulate 准备、启动和正常清理，运行前不得先 start；不要不带文件过滤运行所有浏览器场景，各场景的现场要求不同。

## 执行配置管理

全局入口 `/settings/execution-configs` 支持本机 Codex、本机 Orca/Codex、Pi、DSH（Windows/amd64）、已配对远端 Orca/Codex 的配置编辑、复制、默认选择及条件保存。Web 后端直接查询原生目录；`execution.json` 是唯一配置文件，Go 执行服务只消费启动快照。保存后通过既有原生命令 `steerline restart --no-open` 生效；浏览器不会自动重启。目录结果始终标明原生默认账号范围，不等于项目执行资格。冻结调试包拒绝保存，换方案须生成新包。

定向验证：

```powershell
go run ./cmd/test-feedback -level=L3 -suite=configuration -scenario=configuration.execution-profiles -env=controlled
./scripts/test-execution-profile-discovery.ps1 -SteerRunnerExe '<固定Runner绝对路径>' -OrcaEnvironment '<已登记环境ID>'
```

前者使用独立 controlled 现场覆盖保存/正式重启/CLI消费；后者内部先生成新调试包，只验证真实目录查询并正常清理，不运行模型任务。结果见[专属验收](../docs/acceptance/web-execution-config.md)。

本机 Orca 目录由 Web 自有 named pipe 客户端查询，仅允许 status/agents/modelCatalog，无 Runner 探测接口。本机执行所需 helper 路径/SHA 来自部署模板，缺材料不阻止查询目录；Pi/DSH 无 Orca 结构化目录时可手动填 schema 合法值，并保持未验证标识。helper 的正式发布材料准备与新包复制方式见 Spec，页面不能任意配置程序。
