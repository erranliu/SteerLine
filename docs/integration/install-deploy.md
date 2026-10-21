# 本机安装与一键部署

当前安装目录为 `C:\SteerlineInstall`。双击该目录 `start.bat` 启动后端、独立 Web 和各项目的 Runner 常驻服务，并打开页面；`restart.bat` 用原固定 Runner 重启，保留原任务、Run、PR 和远端终端。`stop.bat` 仍报告原生资源是否完成结算。Web 地址为 `http://127.0.0.1:18742`。配置、数据库、Runner 状态和日志都归属该安装目录，业务 Git 仓库仍在原项目路径。

## 从源码更新

需要同时构建 SteerLine、Web 和 Runner 时，双击根目录 `build-deploy-all.bat`。该入口复用 `build-deploy.bat` 并固定传入 `-UpdateRunner`，其余参数原样转交，默认完成构建、部署和启动；换 Runner 构建须通过原状态的兼容接管核验。

双击 SteerLine 仓库根目录 `build-deploy.bat`。已有安装默认复用 `execution.json` 固定的 Runner EXE，只构建 SteerLine 和 Web；首次安装或显式 `-UpdateRunner` 才从相邻 `..\SteerRunner` 源码构建 Runner。构建成功后停止应用，校验停止或同构建重启资格，安装 EXE 并重新启动。每次记录保存在 `<home>\deployments\<时间-随机ID>\deployment.json`，最新记录为 `<home>\deployment.json`，包含源码版本和三个 EXE 的实际摘要。

需要 Windows、PowerShell 5.1、Go、Git、Node/npm；使用本仓当前工作区，显式构建 Runner 时才读取其源码工作区，不自动 pull/rebase/checkout。构建包括 Web 的 npm ci/build。运行已安装程序不需要 Go、Node 或 Python；实际任务仍需配置所引用的 Git、Codex、gh 和已有机器登录。构建源中允许未提交修改，部署记录标明 dirty；源码与构建依赖变更应先做必要验证。

```powershell
# 自动化调用，不等待按键
.\build-deploy.bat -NoPause
# 只更新，不启动
.\build-deploy.bat -NoStart -NoPause
# 显式构建 Runner，并核验原状态是否允许接管
.\build-deploy-all.bat -NoPause
# 覆盖源码位置或目标安装目录
.\build-deploy.bat -RunnerRepo C:\Project\SteerRunner -InstallHome C:\SteerlineInstall -NoPause
```

脚本只停止与更新显式目标安装目录，拒绝与源码、保留的旧安装目录 `C:\SteerlineRuntime` 或调试目录重叠。所需 EXE 全部准备成功后才停止服务；停止及同构建重启资格均无法核实，或仍有目标进程时不替换。Windows 短暂占用已停机的 EXE 时，仅对文件安装最多重试四次，每次前确认目标未重新运行；不重试任务操作。部署过程持有独占锁；失败保留构建包与错误，不宣称自动回滚。替换中途失败时先修复再重跑，不能仅凭某个 EXE 存在判断整包成功。默认启动后继续等候全部项目 ready，不能仅凭宿主启动成功报告完成。

已有 `execution.json` 必须为 v3，保留默认方案与全部 profiles，只更新 Runner 固定信息；首次空安装只生成本机 Luna medium 默认方案，项目配置需另行提供。脚本不清空、搬运或升级数据库，不重复导入任务，不复制认证材料。可用以下命令查看状态：

```powershell
$env:STEERLINE_HOME='C:\SteerlineInstall'
& "$env:STEERLINE_HOME\bin\steerline.exe" status --json
```

在有 kill-on-close Job 限制的自动化宿主中，直接启动可能明确拒绝常驻；请从普通桌面终端/安装目录启动，不关闭隔离检查或以前台服务冒充常驻成功。

## 同构建更新与重启

显示更新不要求终止原 Orca 执行。同构建重启核对原宿主/Web/Runner 服务退出、项目关闭/锁释放、原构建/冻结配置/checkpoint/候选/检查绑定，保留资源未结算诊断。换 Runner 构建时，候选通过公开 `service verify-takeover` 核验原状态，部署另行证明原服务退出与本地关闭；原 Run、输入、PR 与交付事实保留，资源未结算诊断不改成成功。`restart.bat` 或 `bin/steerline.exe restart --no-open` 使用相同规则。实施与验证见[兼容接管 Plan](../plans/service-generation-recovery.plan.md)。

首次从尚未提供 `service verify-takeover` 的旧 Runner 更新时，在线预检会返回 `service_unavailable: The service query response is unavailable.`，随后部署报告 `Candidate cannot verify original Runner contracts`。此时尚未停服或替换文件。先在普通桌面终端正常停止安装版，再重新执行完整部署，使候选在原写锁释放后走已有的离线兼容核验：

```powershell
$env:STEERLINE_HOME='C:\SteerlineInstall'
& "$env:STEERLINE_HOME\bin\steerline.exe" stop
if ($LASTEXITCODE -ne 0) { throw '原安装停服未核实，保留现场并检查诊断。' }
.\build-deploy-all.bat -NoPause
```

不删除 pin 或数据库来跳过核验；离线报告不兼容时仍不替换。本机首次更新的实际处理见[2026-10-20 部署记录](../acceptance/install-runner-takeover-20261020.md)。

Runner 兼容不代表 SteerLine 自身数据库 schema 兼容。项目若报告 `unsupported SQLite schema`，按项目默认规则使用当前格式的新状态；脚本不会自动迁移或清空旧库。用户明确授权清理时，可归档旧状态，再通过正式任务 validate/create 入口在新库创建所需任务定义和依赖，不能直接改 schema 编号冒充升级。

系统重启或宿主被中断后，最新宿主可能没有停机回执，或 `shutdown.json` 仍属于上一实例。部署由 Go prepare-deployment 按 update 目标组织停止、verify-restart 及必要时一次受控恢复：核实原宿主/Web/Runner 已退出、启动身份、原安装三个 EXE 摘要、Runner pin、冻结配置/checkpoint/项目集合、实际锁及原记录的候选接管证据，再使用原安装运行至全部项目 ready，正常停服并重新执行完整 `verify-restart`。通常直接重跑部署即可，无须手工恢复原版本。

恢复全过程持有启动协调锁；不强杀、清库、改写原 Run/输入/PR 或循环重试。匹配当前实例的失败回执、未知创建、活动服务、文件/配置/身份变化或不兼容仍阻断替换。`resources_settled=false` 保持原事实，旧宿主、旧回执及旧 Runner 诊断随新的 `shutdown-verification.json` 保存。仅允许重启的部署路径使用此恢复，严格资源销毁路径不放宽。实现与隔离验证见[自动恢复验收](../acceptance/deployment-interrupted-recovery.md)；此前人工恢复的现场证据见[2026-10-21 维护记录](../acceptance/install-shutdown-recovery-20261021.md)。

## 本次迁移

三份项目 `SteerLine`、`SteerlineTest`、`merge-h-308-code` 均迁入新目录；保留原 Git 路径、远端、目标分支、任务编号、Spec/Plan 固定引用和依赖。移除旧项目字段 `web_address`，Web 地址由 `web.json` 管理；原三份本机 Codex profiles 转为 v3，未保留已放弃的 Orca 路径。

经用户确认，只迁移任务定义和依赖：新任务全部为 `open`，调度保持 manual + paused。旧交付和执行状态不能作为真实 Runner Run 导入，因此在 `C:\SteerlineInstall\migration\20261015-task-definitions` 留档；旧目录完整保留，已停止。该目录含导入批次、原状态、历史交付证据、旧 Workflow/PR 引用和导入回执；完整旧执行日志仍在旧安装目录。

恢复派发前先按历史处理已交付或等待合并的旧任务，避免重做既有工作。旧任务 101 已交付，任务 102/105 在导出时等待 PR 合并；新任务列表不会将这些历史伪装为 Runner 执行。迁移与部署验证见[验收记录](../acceptance/install-cutover-20261015.md)。

部署判定与脚本收口的当前增量见[简化验收](../acceptance/reliability-simplification.md)：脚本只调用 prepare-deployment 一次，以本次随机调用 ID、完整暂存包摘要、home/目标及 Runner SHA 核对结果。原停止、候选兼容与资源事实保留在 shutdown-verification.json 中。该报告不是持续持有的 writer lease；直接离线替换和中断续装尚未启用。
