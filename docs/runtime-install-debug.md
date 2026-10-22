# 本机安装与独立调试

日常安装目录为 `C:\SteerlineInstall`，包含 `bin\steerline.exe`、`bin\steerline-web.exe`、`bin\steerrunner.exe`。原 `C:\SteerlineRuntime` 停机保留。源码 `build.ps1` 只构建源码目录；根目录 `build-deploy.bat` 构建三个 EXE 并更新新安装版，见[安装与部署](integration/install-deploy.md)。临时调试、AI 自动调试、真实 PR 调试都使用下面的统一入口，不使用或修改任何安装环境。

## 生成

```powershell
.\scripts\new-debug-release.ps1 -SteerRunnerExe C:\Tools\SteerRunner\steerrunner.exe
```

生成只有单一流程：复制仓库 `debug-template/`，构建本次业务/Web/helper，初始化自己的 `projects\SteerlineTest` Git 仓库和任务定义，写入版本记录。默认输出 `C:\SteerlineDebug\dev-yyyyMMdd-HHmmss-<短commit>[-dirty]\`，时间为北京时间、精确到秒，按目录名称升序即可按创建时间排列。成功 stdout 返回 JSON，包括 root/start/stop；已有目录、源码/安装目录重叠和目录链接被拒绝。失败目录保留日志、manifest 标为 failed，不可运行。

数据库按产品 Git common-dir 身份派生到 `home\state\SteerlineTest-<hash>\steerline.db`，不从安装版或模板复制已有数据库。初始任务来自 `seed\tasks.json`，由正式 task validate/create 创建；默认 A → B。生成过程不调用模型，不发布分支或 PR。生成需要 Go、Git、Node/npm 和已固定的独立 Runner EXE；运行不需要 Node、Go 或 Python。默认包含 Web，地址为 `127.0.0.1:18742`，可用 `-WebAddress` 指定另一固定端口；端口冲突明确失败。Runner 来源和安装说明见[Runner 接入](integration/steerrunner.md)。源码及复制源更新后重新生成，已有包保持自己的程序和配置。

## 执行方案

`debug-template/home/execution.json` 是执行方案复制源，默认 live，含显式 simulate；AI 使用 `--simulate` 显式选择模拟方案。模型、effort、worker 配置在这里维护。对应项目/Git/PR 配置由 `home/config-source/SteerlineTest.live.json` 和 `.simulate.json` 维护，路径占位符只在生成时替换。运行封装只选择已有方案、解析工具位置，不拼装第二套执行参数。包内 `home/execution.source.json` 保留完整生成配置；首次选择后，活动配置只暴露所选方案，防止混用 provider。

```powershell
& 'C:\SteerlineDebug\<版本目录>\start.bat'


# AI 模拟调试：用另一套新生成的环境
& 'C:\SteerlineDebug\<另一个版本目录>\start.bat' --simulate

```

默认 live 使用真实 Codex、GitHub `erranliu/SteerlineTest`。首次 live 准备只复制 Codex auth.json 并建立隔离 CODEX_HOME，从 gh 获取本包 token；不复制用户会话或安装版配置。启动时将最小 fixture 推送到 `codex/debug/<本次版本>/base`，不修改 main；候选发布脚本创建针对该 base 的真实 draft PR。包内认证是私有运行材料，不分享或纳入 Git。

simulate 使用包内 helper 模拟 app-server/PR 事实，同时执行真实 Git 提交、HTTP 推送、SQLite、正式任务控制和交付核验；不请求模型或真实 GitHub。模拟材料由 `seed/provider.json` 维护。AI 可使用 `bin/debug-helper.exe debug control --simulate -config <本包配置> -action resume_dispatch|start_task -profile simulate [-task-id 1]` 提交正式控制，并在 PR 发布后通过 `debug merge --simulate` 模拟真实本地 Git 合并与 provider 回执。`debug snapshot --simulate` 返回公开项目快照。

首次运行冻结 live/simulate。未完成的准备可以用同方案重试，换方案必须创建新包。`start.bat` 通过正式统一入口启动后端与 Web 并打开浏览器，调试启动窗口负责持有 Git/provider helper；`stop.bat` 自动读取已冻结方案，正常停止产品并结束本次辅助进程。Ctrl+C 也请求正常清理。交互窗口与自动进程使用同一入口；Web 浏览器关闭不会停止项目服务或 worker。

项目文件、Git 数据、配置、数据库、应用日志与证据位于本包；Runner 执行状态/工作区/日志在本 home 派生的本地卷根短目录，多个项目共用一服务和目录，Git/Codex/gh/Windows 属于机器基础工具，live 的 GitHub/模型属于明确外部服务。路径链接和 Git alternates 被拒绝，不回落到安装版 exe。生成目录固定位置使用，不承诺已有运行目录移动后的可用性。

## 无备份重置

正常停止本环境的项目、应用、Web 服务和 worker 后，双击 `reset-SteerlineTest.bat`，或：

```powershell
.\reset-SteerlineTest.bat --check --no-pause
.\reset-SteerlineTest.bat --no-pause
```

重置不做备份。所有任务恢复 open，清除领取、执行、决策、交付和本包控制状态，恢复 manual 且未暂停。保留任务编号、内容、契约、批次、依赖、Git、PR/provider 事实、认证、已选方案、应用 Runner 构建 pin 和整个共享 Runner 短目录。reset 先取得共享服务全部 Runs/Checks 的停止证明并核对所有项目，再修改 caller 状态；无项目数据库也不能绕过服务核验。新操作分配新的 Runner 请求号，不复用历史回执。活动项目/应用/Web/launcher 锁或不能证明已退出的 worker 阻止重置；不强行结束进程，不支持旧 schema。浏览器 IndexedDB 不由 reset 清除，沿现有实例/版本守卫处理。需要完整初始现场时重新生成。

## 验证

```powershell
./scripts/test-debug-release.ps1 -SteerRunnerExe <absolute-exe>
# 完整 L2：受控门槛后真实 Runner、本机 Codex、实现/报告/发布、真实 PR 合并及交付
./scripts/test-l2.ps1 -SteerRunnerExe <absolute-exe>
```

调试包生成/冻结/停止/reset 由第一条验证，默认产物、单项目和双项目三个新包分别取证；其 `-Live` 复用完整 L2。真实链路只使用本次 SteerlineTest base/head 和已有外部登录，不复制认证或安装数据。原生沙箱、usage、审核及恢复专项已归 SteerRunner，不再运行第二条双任务沙箱验收。历史证据见[原记录](acceptance/codex-sandbox-git.md)，当前要求及结果见[测试精简](acceptance/runner-test-convergence.md)。

长期隔离保留 source-build/path/service-write/dual-home 四项 controlled 场景；工具进程发起时使用 `scripts/test-runtime-isolation.ps1` 的既有独立上下文。浏览器 smoke 使用 `scripts/test-web-smoke.ps1`，同样使用独立上下文，详见 [E2E 指南](../tests/e2e/README.md)。

## 安装版更新

双击仓库根目录 `build-deploy.bat`：先完成构建，再通过正式 stop 结算并停止目标安装版，替换三个 EXE、刷新 Runner 固定版本与 SHA256，最后启动。停止失败则不替换；应用当前停机格式使用一份共享 Runner 结算和各项目本地关闭/锁结果，缺证据均失败。跨 Runner 构建不复用已绑定的旧 home；正常更新保留项目配置、任务库和执行配置中的 profiles，不自动迁移旧 schema。此操作独立于调试环境生成，不用调试包接管安装版活动执行。首次跨 schema 的任务定义迁移与历史留档见[迁移验收](acceptance/install-cutover-20261015.md)。

## 独立 Web 与双项目包

运行 `./build.ps1 -Web` 构建 `bin/steerline.exe` 和 `bin/steerline-web.exe`，只有 Web exe 嵌入页面；普通 `./build.ps1` 无 Node 前置。分别执行两份 exe 的 `install`，目标均为显式 `STEERLINE_HOME/bin`，运行中的对应目标拒绝替换；一份成功不代表另一份已安装。`steerline launchers` 安装统一 `start.bat` / `stop.bat`。旧生成模板仅在内容完全匹配时清理，自定义 bat 保留。

安装目录双击 start 即可启动共享后端、Web 并授权打开。后端冻结装载 `etc/*.json`，每项目独立 actor、数据库、实例、日志与锁，多个项目共享同一后端 PID。应用 v2 在进程内调用原项目用例，无项目 HTTP listener。项目添加和热加载尚未实现，修改配置后正常停启宿主。

`<home>/web.json` 固定 Web 地址，冲突明确失败，不自动换端口。`steerline status --json` 查看宿主/Web/项目状态；`steerline stop` 或 stop.bat 覆盖全部项目，核验逐项目关闭、资源结算、精确进程退出与租约。低层 `steerline serve` 仅供前台/自动化宿主；Web 可单独 stop/serve/open，保持业务和 worker 活动轮次。

生成 `./scripts/new-debug-release.ps1 -WebProjects 2` 取得同一 home 下两份独立 SteerlineTest clone/common-dir/provider；默认 live，AI 显式 `start.bat --simulate`。真实 Edge 自动入口为 `./scripts/test-unified-startup.ps1 -Package <全新双项目包> -Simulate`；查询缓存使用另一个新包并加 `-Scenario query-cache`。模型/PR 协议属于模拟，Go、Git、SQLite 和 Edge 仍为真实实现。结果与清理见[统一启动验收](acceptance/unified-startup.md)。
