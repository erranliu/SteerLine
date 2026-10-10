# 本机安装与独立调试

日常安装固定为 `C:\SteerlineRuntime`，程序为 `bin\steerline.exe`。源码 `build.ps1` 只构建源码目录，不更新安装版。临时调试、AI 自动调试、真实 PR 调试都使用下面的统一入口，不使用或修改安装环境。

## 生成

```powershell
.\scripts\new-debug-release.ps1
```

生成只有单一流程：复制仓库 `debug-template/`，构建本次产品/helper，初始化自己的 `projects\SteerlineTest` Git 仓库和任务数据库，写入版本记录。默认输出 `C:\SteerlineDebug\dev-yyyyMMdd-HHmmss-<短commit>[-dirty]\`，时间为北京时间、精确到秒，按目录名称升序即可按创建时间排列。成功 stdout 返回 JSON，包括 root/start/console；已有目录、源码/安装目录重叠和目录链接被拒绝。失败目录保留日志、manifest 标为 failed，不可运行。

数据库按产品 Git common-dir 身份派生到 `home\state\SteerlineTest-<hash>\steerline.db`，不从安装版或模板复制已有数据库。初始任务来自 `seed\tasks.json`，由正式 task validate/create 创建；默认 A → B。生成过程不调用模型，不发布分支或 PR。生成需要 Go、Git；运行不需要 Go 或 Python。源码及复制源更新后重新生成，已有包保持自己的程序和配置。

## 执行方案

`debug-template/home/execution.json` 是执行方案复制源，默认 live，含显式 simulate；模型、effort、worker 配置在这里维护。对应项目/Git/PR 配置由 `home/etc/SteerlineTest.live.json` 和 `.simulate.json` 维护，路径占位符只在生成时替换。运行封装只选择已有方案、解析工具位置，不拼装第二套执行参数。包内 `home/execution.source.json` 保留完整生成配置；首次选择后，活动配置只暴露所选方案，防止混用 provider。

```powershell
& 'C:\SteerlineDebug\<版本目录>\start.bat'
& 'C:\SteerlineDebug\<版本目录>\console.bat'

# AI 模拟调试：用另一套新生成的环境
& 'C:\SteerlineDebug\<另一个版本目录>\start.bat' --simulate
& 'C:\SteerlineDebug\<另一个版本目录>\console.bat' --simulate
```

默认 live 使用真实 Codex、GitHub `erranliu/SteerlineTest`。首次 live 准备只复制 Codex auth.json 并建立隔离 CODEX_HOME，从 gh 获取本包 token；不复制用户会话或安装版配置。启动时将最小 fixture 推送到 `codex/debug/<本次版本>/base`，不修改 main；候选发布脚本创建针对该 base 的真实 draft PR。包内认证是私有运行材料，不分享或纳入 Git。

simulate 使用包内 helper 模拟 app-server/PR 事实，同时执行真实 Git 提交、HTTP 推送、SQLite、正式任务控制和交付核验；不请求模型或真实 GitHub。模拟材料由 `seed/provider.json` 维护。AI 可使用 `bin/debug-helper.exe debug control --simulate -config <本包配置> -action resume_dispatch|start_task -profile simulate [-task-id 1]` 提交正式控制，并在 PR 发布后通过 `debug merge --simulate` 模拟真实本地 Git 合并与 provider 回执。`debug snapshot --simulate` 返回公开项目快照。

首次运行冻结 live/simulate。未完成的准备可以用同方案重试，换方案必须创建新包。`start.bat` 持有本包服务/helper 的进程所有权；Ctrl+C 请求中心停止并结束本次辅助进程。无 stop.bat。交互窗口与自动进程使用同一入口；控制台启服也绑定包内 `home/service-window.bat`。

所有项目文件、Git 数据、配置、数据库、运行状态、日志与证据位于本包；Git/Codex/gh/Windows 属于机器基础工具，live 的 GitHub/模型属于明确外部服务。路径链接和 Git alternates 被拒绝，不回落到安装版 exe。生成目录固定位置使用，不承诺已有运行目录移动后的可用性。

## 无备份重置

关闭本环境的服务、worker 和控制台后，双击 `reset-SteerlineTest.bat`，或：

```powershell
.\reset-SteerlineTest.bat --check --no-pause
.\reset-SteerlineTest.bat --no-pause
```

重置不做备份。所有任务恢复 open，清除领取、执行、决策、交付和控制台恢复请求/草稿，恢复 manual 且未暂停。保留任务编号、内容、契约、批次、依赖、Git、PR/provider 事实、认证和已选方案。活动服务/控制台锁或不能证明已退出的 worker 阻止重置；不强行结束进程，不支持旧 schema。需要完整初始现场时重新生成。

## 验证

```powershell
.\scripts\test-debug-release.ps1
# 显式真实 PR 验证，无模型调用，创建后关闭精确 PR 并删除本次分支
.\scripts\test-debug-release.ps1 -Live
```

两条命令都先通过生成脚本创建两套新包；结果/日志保留在包内 evidence。范围和结果见 [Plan](plans/debug-release.plan.md)、[验收](acceptance/debug-environment.md)。现有 `debug-runtime.ps1` 只用于既有源码隔离回归，不是日常或 AI 调试入口。

## 安装版人工更新

更新安装版前先结算外部 worker/PR/会话事实，停止服务，再替换安装 exe；不要用调试包接管安装版活动执行，不迁移或清空旧数据库。此操作独立于调试环境生成。
