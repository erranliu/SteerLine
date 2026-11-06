# 独立调试包

临时、AI 自动和真实 PR 调试使用源码外的独立包，不修改安装环境。日常安装和更新归[安装部署](integration/install-deploy.md)，Web 单独构建与操作归[Web 指南](../web/README.md)。

## 生成

```powershell
.\scripts\new-debug-release.ps1 -SteerRunnerExe C:\Tools\SteerRunner\steerrunner.exe
```

需要 Go、Git、Node/npm 和固定的独立 Runner EXE。Runner 可由参数、STEERRUNNER_EXE 或工具发现选择，须为有效绝对文件，不能来自安装目录或链接；记录公开版本、协议与 SHA。生成过程不调用模型、不推分支或创建 PR。

生成器复制 debug-template，构建业务/Web/helper，创建本包 Git 仓库，通过正式 task validate/create 初始化 seed/tasks.json（默认 A → B）；不复制安装库。输出默认为 `C:\SteerlineDebug\dev-yyyyMMdd-HHmmss-<短commit>[-dirty]\`，stdout JSON 返回 root/start/stop。已有目录、源码/安装重叠和链接被拒绝；失败目录留日志、manifest 标 failed，不可运行。

并行调试按[路径清单](../SteerlineFlow/paths.md)分配独立 scope/packages 作为 DebugRoot；同源码构建仍须串行。生成后核对 home、端口及 live base 分支隔离。默认生成时选一个空闲 loopback 端口，也可用 -WebAddress 显式指定；选定后写入配置并固定，运行时冲突失败，不自动换端口。源码或模板更新后生成新包，旧包程序不变。

## 执行方案

```powershell
& 'C:\SteerlineDebug\<版本目录>\start.bat'
# 模拟使用另一套全新包
& 'C:\SteerlineDebug\<另一版本目录>\start.bat' --simulate
```

默认 live，首次运行冻结所选方案，准备中断可按同方案续做，换方案须新包。execution.source.json 保存完整生成配置，活动 execution.json 仅暴露所选方案；项目配置来自 config-source 的 live/simulate 模板。运行封装选择既有方案，不再组装一套模型/权限参数。模型等设置与可选 Orca 参数见[生成器](../scripts/new-debug-release.ps1)和[执行配置](product-design/execution-configuration.md)。

| 方案 | 实际效果 |
| --- | --- |
| live | 真实 Agent 与 GitHub SteerlineTest；从已有登录复制 Codex auth.json 到隔离 CODEX_HOME，并从 gh 取得本包 token。fixture 推送本包 codex/debug/<版本>/base，候选创建针对该 base 的真实 draft PR，不改 main |
| simulate | helper 模拟 app-server/PR，真实运行 Git、HTTP、SQLite、正式任务控制和交付核验；不请求模型或真实 GitHub，模拟材料来自 seed/provider.json |

包内认证属于私有运行材料，不分享或纳入 Git，不复制安装配置或用户会话。两种方案都通过正式统一启动入口运行后端和 Web；调试启动窗口持有 Git/provider helper，stop.bat 自动读取冻结方案并正常清理，Ctrl+C 也请求清理。关闭浏览器不停止业务。

项目/Git、配置、数据库和证据在包内；Runner 状态、工作区和日志在本 home 派生的短目录，同 home 多项目共享一个服务。机器 Git/Agent/gh/Windows 与 live 外部服务是显式依赖。链接、Git alternates 和安装版 EXE 回退被拒绝；已有运行目录固定位置使用，不承诺移动后可用。

## 无备份重置

先正常停止本包项目、应用、Web、Runner 及辅助进程，再执行本包入口：

```powershell
.\reset-SteerlineTest.bat --check --no-pause
.\reset-SteerlineTest.bat --no-pause
```

重置不备份：全部任务恢复 open，清除调用方领取、执行、反馈、交付与控制状态，恢复 manual 且**未暂停**。保留任务定义/编号/契约/批次/DAG、Git、PR/provider 事实、认证、所选方案、Runner pin 和共享 Runner 历史目录；新操作使用新请求号。

先核对全部项目及共享 Runner 所有 Run 的停止证明，即使项目数据库缺失也不能跳过。活动锁或无法证明退出的资源阻断，不强杀、不兼容旧 schema。浏览器 IndexedDB 不清除，沿原实例/版本守卫处理。需要完整初始现场时重新生成。具体边界见[重置实现](../tests/e2e/internal/driver/debug_reset.go)。

## 验证入口

| 目的 | 入口与范围 |
| --- | --- |
| 生成、冻结、停止与 reset | scripts/test-debug-release.ps1；默认产物、单项目和双项目分别用新包 |
| L4 真实交付 | scripts/test-live-delivery.ps1；真实 Runner/本机 Codex/PR/合并，只使用本包 base/head 和已有登录 |
| 隔离与浏览器 smoke | scripts/test-runtime-isolation.ps1、scripts/test-web-smoke.ps1；使用既有独立上下文 |
| 双项目与查询缓存 | new-debug-release.ps1 -WebProjects 2；test-unified-startup.ps1 -Package <全新包> -Simulate，缓存场景另用新包并加 -Scenario query-cache |

验证脚本的 Runner 参数使用固定绝对 EXE；测试预算和环境边界归[E2E 指南](../tests/e2e/README.md)。本页列入口不表示已执行。沙箱、usage、原生审核/恢复专项归 Runner；当前范围见[测试精简验收](../SteerlineFlow/testing/acceptance/runner-test-convergence.md)，[原沙箱记录](../SteerlineFlow/testing/acceptance/codex-sandbox-git.md)仅为历史。

## 独立 Web 与双项目包

-WebProjects 2 生成同 home 下独立 clone/common-dir/provider，后端共享 PID，项目分别持有 actor、数据库、实例和锁；Runner 共享。模拟场景中 Go、Git、SQLite、Edge 仍为真实实现，模型/PR 协议由 helper 模拟。结果见[统一启动验收](../SteerlineFlow/testing/acceptance/unified-startup.md)。

应用配置位于 home/etc/*.json，宿主启动时冻结装载；项目添加与热加载尚未实现，修改后正常停启。Web 地址归 home/web.json；status/stop 与独立 Web 操作见[Web 指南](../web/README.md#构建与启动)。精简前安装/调试说明和[历史迁移验收](../SteerlineFlow/testing/acceptance/install-cutover-20261015.md)统一从[历史来源](../SteerlineFlow/archive/design/runner-reliability-history.md#第五批整理来源)追溯。
