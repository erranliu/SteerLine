# 有人值守的远程执行 MVP

> 本页记录旧 minimal-v1 远端试验，不是当前 worker-flow-v1 的操作说明。Orca 本机和受监管远端的适配代码与平台探针保留，但当前生产派发在首次领取前拒绝这两种目标；功能交付与验收已推迟。

当前最小交付使用一个本机 SteerLine 中心，通过 Orca 在已认证的 Windows 远端运行 Codex。初始交付目标为台式机64g，中心 Orca 1.4.209、远端 1.4.215，命名方案固定 `gpt-6-luna` / `xhigh`。远端不运行 SteerLine 服务。使用范围和门槛见 [Spec 03](spec/03-remote-execution-and-viewer.md#有人值守-mvp-交付边界)。

## 使用

1. 保持两端 Orca 在线，远端 Git、Codex、项目检查工具和 PR 发布凭据可用。本机项目配置指定的预检终端须保持打开；每个新运行另有由中心创建的独立协调终端。
2. 打开 `C:\SteerlineRuntime\service-window.bat`；SteerlineTest 使用参数 `C:\SteerlineRuntime\etc\SteerlineTest.json`。保持服务窗口打开。
3. 打开 `C:\SteerlineRuntime\console.bat`，选择 Ready 任务，按 `s`，选择“Orca 台式机64g Codex Luna xhigh”并确认。已有本机配置和自动派发默认配置保持不变。
4. 中心控制 prepare、implement、verify、review、publish_pr；在控制台回答业务问题，在代码托管平台审阅并人工合并 PR。中心核对固定 head 的真实合并后才关闭 Task、释放 DAG 后继。

安装中的 SteerLine、SteerlineTest 使用独立的远端仓库 selector、协调终端及本机状态目录。旧 Beads 项目 merge-h-308-code 保持只读，不迁移或派发。

## 能力与限制

- MVP 是人工选择方案并值守的远程执行；每任务只有一个活动候选。当前主机的 Decision 与步骤转换样本已通过 4 秒 viewer 门槛；不承诺无人值守长期可靠性或未认证主机的时延。
- 必须保留能力报告及其 SHA-256 绑定的证据文件。已测主机的六项基础能力与每个项目的仓库/步骤/工具预检分别核验；换主机不能直接复制旧主机证据。配置存在不代表通过领取门槛。
- 模型与思考强度固定在每次 WorkflowRun 中；配置变更不重定向已开始的任务。服务和控制台在修改配置后重开。
- 不确定的启动、回答或发布结果保留原 Operation、Attempt 和资源占用，先对账，不重复启动或创建 PR。重启中心仍使用原 home 和 SQLite。
- 远程取消不可用。关闭 viewer 不停服；关闭服务窗口只停止中心监督，远端 worker 可能继续。不要以关窗作为已停止证明。已结算的 worker 正常 release。
- Orca 重启或专用协调终端丢失时，旧句柄不再可靠。先恢复并核验已有执行，再为后续任务更新协调终端；不能通过新句柄接管身份不明的活动执行。

当前台式机64g 的目标功能、受影响故障与 4 秒 viewer 已按[Spec 03 最终范围](validation/spec03-final-scope-audit-2026-09-28.md)核对；原中心 #3/#4/#6 TaskDelivery 均已关闭。Orca 平台自然产生同句柄新 incarnation 未被声称已观察，出现时仍须定向回归。长期无人值守、固定 stable 24 小时与受控提升仍属于 #32；#29/#32/#24 的其他硬前置保持。

## 发布与验证

发布使用固定源码构建，在确认无活动/未知执行后停止项目服务，备份二进制、配置与当前状态，调用构建产物的 `install` 写入同一 home，再以手动模式重启。原生任务库不清空、不转换、不重建。安装目录的发布记录保存源码提交、二进制哈希、项目配置和验证结果；备份只用于人工恢复现场，不提供旧数据兼容路径。

基础五步与命令式发布证据见[台式机64g 原生验收](validation/spec03-desktop64g-2026-09-28.md)。交付配置另核对远端模型可启动、项目工具与仓库身份、能力证据可重读、现有任务/Decision/Operation 保留及重启后的服务配置。

本次本机安装、项目方案与状态保留结果见[发布回执](validation/remote-mvp-release-2026-09-28.md)。
