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

在不同终端按顺序运行 `steerline serve-app`、`steerline-web serve`，然后 `steerline-web open [-project <id>]`。项目服务由已有 `start-service`/`service-window.bat` 独立启动；Web 不启动它们。`steerline-web stop` 与 `steerline stop-app` 正常停止各自精确实例并核实退出/租约释放，不停止项目服务或 worker。

Web 地址保存在 `<home>/web.json`：`{"version":1,"address":"127.0.0.1:18742"}`；缺文件时采用该默认值并在启动时保存。稳定端口冲突明确失败。项目配置的 `web_address` 和旧 `steerline web -config` 已删除，旧配置需要显式移除该字段，不自动迁移。

当前项目使用 `/projects/{project_id}/dag`、`/runs`、`/settings` 深链接。切换器显示仓库位置、状态和诊断，可搜索；每标签页的选择独立。最近选择可共享，不能抢改已打开的标签页。配置目录来自 `<home>/etc/*.json`，坏配置保留诊断，同 common-dir 多配置拒绝路由；列表和离线查询不初始化业务数据库。

## 认证与恢复

`open -print-url` 仅供受控浏览器自动化，原生 launcher 从当前用户私有 discovery 申请一次性 60 秒票据。票据在 fragment，页面兑换前移除；12 小时会话绑定 home/Web 实例，HttpOnly/SameSite Strict、精确 Origin/Host、CSRF 和生产 CSP 保留。应用与项目 Bearer 不进入浏览器。

Web 重启后再次 `open` 授权。应用服务重启后 Web 核对同 home 的新实例，再恢复只读连接。草稿与完整原请求存 IndexedDB，按 home/项目/实例及执行身份隔离；切换取消旧查询、资格检查和未发送回调。已发送命令只按原 ID 查询；原实例变化后仍可读取旧回执，但不把旧信封补交到新实例。离线只保留当前标签页已有缓存并禁写。

## 独立调试与验证

```powershell
./scripts/new-debug-release.ps1 -WebAddress 127.0.0.1:18742 -WebProjects 2
& 'C:\SteerlineDebug\<新包>\start.bat' --simulate
& 'C:\SteerlineDebug\<同一包>\web.bat'
```

默认 live，AI 显式 `--simulate`；首次方案冻结。生成器维护双 exe、配置和启动器，两项目使用独立 clone/common-dir/provider，配置源在 `home/config-source`，etc 仅保存活动项目。包内 `reset-SteerlineTest.bat` 同时检查两项目、应用、Web 与控制台锁，再重置本包执行状态。不使用安装目录准备或运行调试。

热更新：先在 `web/` 运行 `npm run dev`，为 Vite 设置 `STEERLINE_WEB_DEV_PORT=18742`；启动本包 Web 前设置 `STEERLINE_WEB_DEV_ORIGIN=http://127.0.0.1:5173`。浏览器始终通过 Go 的固定 Origin，只有静态/Vite HMR 路由转到 Vite；认证和 API 留在 Go。

```powershell
npm --prefix web test
npm --prefix web run build
go run ./cmd/test-feedback -- ./internal/app ./internal/product/applicationhttp ./internal/product/applicationclient ./web/server -run '^TestWebSwitch' -count=1
go run ./cmd/test-feedback -level=L1
```

真实 Edge 切换场景会启动和正常清理全新双项目包：设置 `STEERLINE_WEB_PACKAGE=<新包>`，运行 `npm --prefix web run test:e2e -- project-switch.spec.ts`。安装检查入口为 `./scripts/test-web-switch-build.ps1 -Package <新生成的 Web 包>`，也只使用包内 home。

`./scripts/test-web-smoke.ps1` 可自动生成另一新单项目包、启动、运行原 smoke 并正常清理。手动入口的原 `web.smoke` 使用另一个全新单项目 Web 包，先 `start.bat --simulate`，设置 `STEERLINE_WEB_CONFIG=<包>/home/etc/SteerlineTest.json` 后运行 `npm --prefix web run test:e2e -- smoke.spec.ts`；收尾显式正常停止 Web、应用和项目，核实本包进程/监听为 0。截图、trace 和原请求/轮次身份记录在 `STEERLINE_WEB_EVIDENCE` 指定的证据目录或默认临时目录。模拟范围仅限外部模型/PR 协议，Go、Git、SQLite 和 Edge 均为真实实现。
