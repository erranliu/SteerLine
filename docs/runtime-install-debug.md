# 本机安装与源码调试

Windows 日常安装默认使用 `C:\SteerlineRuntime`：程序位于 `bin\steerline.exe`，配置位于 `etc\`，项目状态和工作区位于该 home 下。仓库的 `build.ps1` 只写当前源码树的 `bin\steerline.exe`，不会更新日常安装。

## 源码调试

调试需要一个与日常安装不重叠的绝对 home，以及放在该 home 内的项目配置。配置中的显式 `run_dir` 和 `worktree_root` 也必须解析到调试 home 内；项目仓库应是独立 clone，放在 home 外。`scripts/debug-runtime.ps1` 会拒绝重叠路径和缺失的构建产物，不会回落到 PATH 或日常安装。

```powershell
$repo = 'C:\src\SteerLine'
$debugHome = 'C:\SteerLine Debug'
$installedHome = 'C:\SteerlineRuntime'
$config = Join-Path $debugHome 'etc\SteerLine.json'

& (Join-Path $repo 'scripts\debug-runtime.ps1') -DebugHome $debugHome -InstalledHome $installedHome -Config $config -Operation build
& (Join-Path $repo 'scripts\debug-runtime.ps1') -DebugHome $debugHome -InstalledHome $installedHome -Config $config -Operation install
& (Join-Path $repo 'scripts\debug-runtime.ps1') -DebugHome $debugHome -InstalledHome $installedHome -Config $config -Operation launchers
& (Join-Path $repo 'scripts\debug-runtime.ps1') -DebugHome $debugHome -InstalledHome $installedHome -Config $config -Operation console
```

`build` 编译仓库产物；`install` 将该产物复制到调试 home；`launchers` 在同一 home 生成入口。`console`、`service` 和 `cli` 只运行该调试副本，并绑定传入的调试 home 与配置。调试入口把 `CODEX_HOME` 设为调试 home 下的 `codex` 目录；若该目录没有 `auth.json`，会从指定的 `CodexAuthHome`（默认当前 `CODEX_HOME`，否则用户 `.codex`）复制认证文件，不复制会话或其他可写状态。重复运行 `launchers` 可重新生成入口。调试 home 下的 console 会显示实际 exe、home、配置、源码 commit 和 dirty 状态；未知构建信息明确显示为未知。

更新源码后先停止使用该调试 exe 的服务，再运行 `build` 和 `install`；安装副本正在运行时，Windows 会拒绝替换。可在此期间继续使用日常安装 home。调试入口的目录边界保护同时覆盖显式状态和工作区路径。

## 重启与人工更新

同一构建重启是停止并重新启动服务，继续使用原 exe、home、配置和当前格式状态。它不替换二进制，也不表示跨构建接管活动任务。

人工更新日常安装前，先核实并结算原外部执行及其 PR/会话事实；中心服务停止本身不会结束外部 worker。结算完成后停止中心、替换 `C:\SteerlineRuntime\bin\steerline.exe`、需要时重新运行 `launchers`，再以原 home 和配置启动服务。不要把旧数据库迁移、清空、回退或用新构建接管活动执行当作更新步骤。
