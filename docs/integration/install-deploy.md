# 本机安装与一键部署

日常安装 home 为 `C:\SteerlineInstall`。start.bat 启动共享后端、独立 Web 和同 home 的一个 Runner 服务；restart.bat 按原绑定核验重启，stop.bat 正常关闭并报告资源结算。Web 默认 `http://127.0.0.1:18742`，配置归 web.json，冲突明确失败。

配置、项目数据库和应用日志归 home；Runner 状态/工作区/日志位于由 home 派生的短目录，默认卷根下 `C:\slr\<hash>`，同 home 多项目共享。业务 Git 仓库留在配置路径。身份与路径规则见[Runner 接入](steerrunner.md)，Web 单独构建、认证和启动见[Web 指南](../../web/README.md)，隔离验证使用[独立调试包](../runtime-install-debug.md)。

## 从源码更新

| 入口 | Runner 选择 |
| --- | --- |
| build-deploy.bat | 已有安装复用 execution.json 固定的 EXE；首次空安装才从相邻 SteerRunner 源码构建 |
| build-deploy-all.bat | 固定传入 -UpdateRunner，构建 SteerLine、Web 与 Runner |
| build-deploy-discard-runs.bat | 固定传入 -UpdateRunner -DiscardRuns，明确放弃目标 home 全部旧执行，效果见下节 |

需要 Windows、PowerShell 5.1、Go、Git、Node/npm；使用当前工作区，不自动 pull/rebase/checkout。构建允许 dirty，但记录源码版本与 dirty。安装版运行不需要 Go、Node 或 Python，任务使用的 Git、Agent、PR 工具及登录仍须有效。

```powershell
.\build-deploy.bat -NoPause
.\build-deploy.bat -NoStart -NoPause
.\build-deploy-all.bat -RunnerRepo C:\Project\SteerRunner -InstallHome C:\SteerlineInstall -NoPause
.\build-deploy.bat -ForceWebInstall -SerialBuild -NoPause
```

-NoPause 不等待按键；-NoStart 只安装，不证明服务已 ready。-RunnerRepo 仅在选择构建 Runner 时使用；-InstallHome 必须是明确独立目录，与源码、Runner 源码、旧安装保留目录和调试目录分离。

默认前端依赖检查/构建与 Go 流水线并行，Go 内按 Runner（如选择）、SteerLine、Web 顺序构建；前端完成后才嵌入 Web。任一失败均不部署，后台构建结束后才释放锁。源码 `bin/.build.lock` 和 Runner 的 `bin/.steerline-build.lock` 阻止同源码重复构建；直接 npm/Go 入口不参与这些锁，仍须遵守[共享路径约束](../../SteerlineFlow/paths.md)。

Web 依赖以 package/锁文件、工具版本、平台和有效 npm 配置等指纹复用成功安装，标记为 `web/node_modules/.steerline-install.json`。失效时 npm ci；这不校验每个依赖文件，手工破坏依赖后用 -ForceWebInstall。npm 配置只参与内存哈希，不将凭据写入报告。

只构建不部署使用 `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build.ps1 -Web`；同样支持 ForceWebInstall、SerialBuild、RunnerRepo、TimingPath。-Release 禁用两个产品 EXE 的[开发者配置](../product-design/execution-configuration.md#开发者配置与环境就绪阻塞)。

## 部署判定与记录

1. 在构建锁内冻结独立 `bin/deploy-builds/<随机ID>/` 包，校验三个 EXE、Runner 版本/摘要和执行配置。全部成功才停止目标安装。
2. 通过一次 prepare-deployment 组织原应用停服与证明，核对本次调用 ID、home、完整暂存包和 Runner SHA；随后执行升级预检。核验失败不替换原文件，服务可能已停止。
3. 确认目标进程退出后安装并核对全部文件摘要，再写配置/部署记录。Windows 短暂占用时仅文件安装最多额外重试四次，每次重新确认目标未运行；任务操作和启动不自动重试。
4. 默认通过 Windows 进程管理接口执行正式 start，保留当前用户环境而不将其写盘，避免继承部署宿主 kill-on-close Job。结果绑定本次调用；全部项目须在 180 秒内 ready，创建进程或单个项目成功均不足。-NoStart 跳过这一步。

过程持有目标独占锁；中途替换失败保留错误和构建包，不承诺自动回滚或中断续装。不能凭某个 EXE 存在判断整包完成；修复原因后重新走正式入口。

已有 execution.json 须为当前 v3，保留默认项和全部 profiles，仅更新 Runner 固定信息；空安装从示例生成 local-luna-medium，项目配置另行提供。普通更新不清库、不重复导入、不复制认证、不迁移旧 schema。出现 unsupported SQLite schema 时按当前格式准备新状态；不能修改 schema 编号冒充升级。

| 记录 | 内容 |
| --- | --- |
| `<home>/deployments/<时间-随机ID>/deployment.json` | 源码版本/dirty、Runner 来源、三个 EXE 摘要；最新副本为 home/deployment.json |
| 同包 shutdown-verification.json、runner-upgrade-preflight.json | 原停止、资源与候选离线能力证明，绑定本次部署；不是持续 writer lease |
| 同包 startup-<调用ID>.log/.json | 本次正式启动输出和退出码 |
| 同包 build-timings.json、timings.json | 构建与部署阶段状态、跳过原因、耗时和失败；并行总耗时为墙钟时间 |

源码报告默认在 `bin/build-timings/`、`bin/deploy-timings/`，-TimingPath 可指定当前入口的绝对路径。早于取得锁的拒绝不保证有报告。已结束调用的独立构建快照可手工清理，不清理仍在使用的包。

## 同构建更新与重启

**同一 Runner 构建**可保留原后台执行，核验原宿主/Web/Runner 服务退出、项目关闭/锁释放、checkpoint 归属和原 Run/input。匹配停机回执及 stopped 检查点允许加载已保存待重启配置，历史执行仍沿原信封。资源未知保留诊断；资源丢失由 Runner 返回 blocked/manual_intervention_required，显式 resume 只做有界原操作对账，不重建或自动补发。

**换 Runner 构建**使用当前 state/v8 排空规则：原服务退出，原/候选公开离线查询核对同一 Run/input 集合，业务终态、原生资源与未完成控制全部核实。仅 pr_ready、仅服务停止或 stop_status 字段不足以升级；不支持活动 Run 跨构建接管，verify-takeover 已删除。普通预检不自动停止 Run、不迁移或清空数据，失败后按原诊断用原版本收尾再重跑。

停止证明须绑定原资源身份与范围：confirmed 的 Orca 终端停止回执可证明原终端已停止，不扩展为后台进程或凭据已清理；accepted/unknown、身份或冻结范围不符仍须核实。本机 Orca 历史 PID 不可查询时，还需原构建停机结算及同范围 Run 已确认停止；发现进程存活或证据缺失则阻断。详细资格归[Runner 接入](steerrunner.md)。

旧 pin 指向不同命名空间（含 v7 及更早）时，先由原版本收尾，改用全新且分离的 InstallHome；不复制旧执行状态，不删除 pin/数据库绕过检查。格式与执行契约兼容且已排空的 v8 构建可按正式核验复用目录，历史构建摘要仅表示来源。

**宿主被中断**而停机回执缺失/陈旧时，普通部署可由 prepare-deployment 一次受控运行原安装、等全部项目 ready 后正常关闭，再重新核验。须证明原进程退出、锁可用、原三个 EXE、Runner pin、冻结配置/checkpoint、项目集合及 Run/input 一致。当前实例失败回执、活动服务、身份/文件变化、不兼容或未知创建继续阻断；不强杀、清库或循环重试，不放宽严格资源销毁路径。恢复保留原诊断与 resources_settled 事实。范围见[自动恢复验收](../../SteerlineFlow/testing/acceptance/deployment-interrupted-recovery.md)。

### 显式放弃旧运行后升级 Runner

只有决定不再继续所选 InstallHome **全部项目旧执行**时使用：

```powershell
.\build-deploy-discard-runs.bat -NoPause
# 等价：.\build-deploy-all.bat -DiscardRuns -NoPause
```

-DiscardRuns 必须与 -UpdateRunner 组合，可加 -NoStart。构建及能力预检完成后，先在独立位置通过正式任务校验/创建入口准备任务定义与 DAG，再核对原应用/Web 退出、checkpoint、项目/仓库身份及锁。已保存待重启的 profile/default/开发设置可与原冻结摘要不同，仓库、remote、分支及 Runner 绑定须一致，准备期间再改配置会阻断。

原固定 Runner 公开冷查询全部 Run/input，先持久保存原 stop 信封并在 worker 启动前受理，沿原号结算、停止服务并核实全部资源。未知停止、未完成恢复授权、输入/项目归属不明、锁或证明缺失均阻断，不把“放弃”当作资源已消失。

通过后将原项目 state、应用/Web 状态及 Runner 命名空间归档到 `<home>/maintenance/discard-runs-<调用ID>/`，切换准备好的任务库，再安装新程序：

- 保留配置/profiles/default、任务编号/内容/固定契约/DAG、业务 Git 和远端 PR。
- **全部任务恢复 open（包括原已完成/放弃），所有项目 manual 且暂停派发。** 旧执行、交付、环境资格、生成、Roadmap 和控制历史仅供归档查证，不恢复为新执行状态。
- -NoStart 不需要临时启动服务或人工导入；恢复派发前须按实际业务选定任务。

原宿主中断且缺停机回执时，可严格离线核验原进程退出、租约/锁、全部数据库 checkpoint 身份及原 Runner 绑定，再由原 Runner 停止资源，不临时重启项目宿主。只允许明确缺失回执；已有但损坏、过期或本地关闭未核实的回执仍阻断。discard.json 保留 interrupted_host 和原停止诊断。

original-shutdown.json、discard.json 记录原请求、结算、任务数量和切换进度；shutdown-verification.json 绑定部署包。开始切换后中断会留下 `<home>/deployment-discard.json`，阻断 start/serve、后续部署及升级预检，须按该记录完成现场恢复；不自动重发未知 stop 或回滚。开始切换前失败不移动原执行目录，服务可能已停止。本功能使用当前 Runner 命名空间，不迁移旧运行状态。schema 41 程序的显式放弃模式允许只读核验 schema 40 的完整结构、任务和原运行归属，归档旧库并重建新 schema 41 任务库，不改版本号或原地升级；正常启动/查询/写入仍只接受 schema 41，其他旧格式或定义不符继续阻断。实际范围见[清理修复验收](../../SteerlineFlow/testing/acceptance/deployment-schema40-discard-20261106.md)。

## 实现与历史证据

当前入口以[部署脚本](../../scripts/build-deploy.ps1)及[可靠性简化验收](../../SteerlineFlow/testing/acceptance/reliability-simplification.md)为准；文档整理不执行安装维护。[2026-10-15 迁移](../../SteerlineFlow/testing/acceptance/install-cutover-20261015.md)、[2026-11-01 v8 安装清理](../../SteerlineFlow/testing/acceptance/runner-v8-install-reset.md)及[原停机恢复现场](../../SteerlineFlow/testing/acceptance/install-shutdown-recovery-20261021.md)仅说明各自授权与原版本事实，不构成日常更新规则。原文及其他来源见[历史入口](../../SteerlineFlow/archive/design/runner-reliability-history.md#第五批整理来源)。
