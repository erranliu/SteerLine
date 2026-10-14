# SteerLine Web

本机浏览器客户端。此目录独立维护 npm 工程、UI、浏览器认证服务（Go 包 `web/server`）、构建和定向测试。业务状态和所有命令仍由现有 Go 服务核验。[Spec](../docs/spec/web-frontend.spec.md) · [Plan](../docs/spec/web-frontend.plan.md) · [验收](../docs/acceptance/web-frontend.md)。

## 构建与使用

```powershell
npm --prefix web ci
npm --prefix web run build
go build -tags webui -o <输出目录>/steerline.exe ./cmd/steerline
```

项目配置设置 `"web_address": "127.0.0.1:18742"`，显式启动该配置的服务。不同项目/包使用不同固定端口；重启同一配置保持地址。普通 Go 构建不需要 npm，但不含页面资源，配置 Web 时会明确报错。

```powershell
steerline web -config <项目配置>
```

此入口在已有实例申请短期、一次性打开票据并打开浏览器，不自动启动服务。会话过期/服务重启需要再运行此入口。`-print-url` 只供受控自动化；票据在 fragment，页面兑换前从地址栏移除。配置和宿主路径都属于服务机器。

## 独立调试

```powershell
./scripts/new-debug-release.ps1 -WebAddress 127.0.0.1:18742
& 'C:\SteerlineDebug\<新包>\start.bat' --simulate
& 'C:\SteerlineDebug\<同一包>\web.bat'
```

不使用安装目录。live/simulate 首次运行冻结，换方案生成新包。生成器在启用 Web 时执行 npm ci/build 并嵌入静态资源；源模板、构建和启动仍沿唯一调试流程。

开发热更新：在 `web/` 运行 `npm run dev`；启动同一独立调试包服务前，设置 `STEERLINE_WEB_DEV_ORIGIN=http://127.0.0.1:5173` 和 Vite 的 `STEERLINE_WEB_DEV_PORT=18742`（实际固定 Web 端口）。浏览器仍通过 Go 的 18742 同源地址访问，Go 只将静态/Vite HMR 路由转接至 Vite，`/auth`、`/api` 始终留在 Go。开发 Origin 必须为显式 loopback；开发时只为 Vite 注入脚本开放 inline，生产 CSP 保持 self。环境变量仅用于本次开发进程，调试包 scheme 仍冻结。

## 持续定向验证

```powershell
npm --prefix web test
go run ./cmd/test-feedback -- ./web/server ./internal/product/httpapi -count=1
# Go 的固定 L1 已登记 quick-owner-web。
go run ./cmd/test-feedback -level=L1
```

真实浏览器场景 `web.smoke` 位于 `tests/`，使用当前 Edge/Chromium。先生成并启动新的 simulate 包，再设置 `STEERLINE_HOME` 为该包 home、`STEERLINE_WEB_CONFIG` 为其 `home/etc/SteerlineTest.json`；运行 `npm --prefix web run test:e2e`。驱动核对目录为 `C:\SteerlineDebug` 下的新包、scheme=simulate，拒绝安装目录。截图/trace 默认写系统临时目录；可将 `STEERLINE_WEB_EVIDENCE` 指向本次新空证据目录。

前端测试 `web.client` 覆盖持久请求、未知回执、权威缺失、跨标签锁及原实例；`web.input` 覆盖中文 IME、Enter 和问题草稿。浏览器测试使用真实服务与数据，不用 MSW/模拟业务响应替代接线。需要产品真实 PR 验收时，单独按 Plan 使用 SteerlineTest 本次专属分支，不在本目录测试默认执行。
