# 2026-10-19 安装版 Runner 更新与执行清理

用户直接授权处理 `C:\SteerlineInstall` 的部署失败，并允许清理任务和过去执行。本次为实际安装维护，不作为调试或模型执行验收；未操作 `C:\SteerlineRuntime`、远端 Git 分支或 PR。

## 处理范围

- 原 Runner 对三个旧 Run 接受显式 stop，随后应用正常 stop 返回 0。原实例 `95e2267018576f4c44f1f30e4a834924` 的停服回执及公开 `service verify-stopped` 均确认 resources_settled。逐 Run 原生证明范围为 `orca_terminal`，不扩展为全执行主机进程或凭据撤销证明。
- 清除三个项目的执行历史、候选、检查、投影与控制状态；20 个任务恢复 open，保留任务定义、契约、批次和依赖。调度设为 manual + paused。
- 旧 Runner 目录、旧绑定与维护证据归档到 `C:\SteerlineInstall\maintenance\upgrade-runner-20261019-9c34`，新 Runner 使用新的空执行状态，不读取旧构建的数据。
- 更新后发现新 Runner 的 conversation/1、generate/1 明确发布空 `prompt_steps`，原调用方将其判为目录不完整。修复为允许显式空列表，继续拒绝缺失列表、空 ID、非法版本、缺适配器及重复工作流。

## 验证结果

| 项目 | 命令或证据 | 结果 |
| --- | --- | --- |
| L1 定向调用方回归 | `go test ./internal/runnerclient -run 'TestSteerRunnerUsesPublishedSchema\|TestOrcaNativePublishedConfiguration' -count=1`；测试位于当前工作区，使用隔离公开契约夹具 | 通过，`ok .../internal/runnerclient 1.221s`；有效空列表和不完整/重复目录反例均覆盖，原公开配置 schema 校验保持有效 |
| 源码格式 | `git diff --check` | 通过 |
| 原资源停止 | 维护目录 `runner-stopped-verification.json`、`application-shutdown.json` | 三个原 Run 已停止；原宿主/Web/Runner 退出，各项目本地关闭和锁释放，资源结算通过 |
| 定义保留与清理 | `reset-result.json`、`definitions-before.json`、`verification.json` | 三库完整性及外键检查通过；定义/契约/依赖逐项不变；20 个任务 open，候选 0，新 Runner Runs 0、Checks 0 |
| 实际部署 | `build-deploy-catalog-fix.log`；`C:\SteerlineInstall\deployments\20261019-133919-5f918c36\deployment.json` | 三份 EXE 安装完成且摘要与部署记录一致；Runner SHA256 `33851d039193ca1f2f6766ab15e55abb54bda118ed673f81f45ad2cc80ec6b49`，源码 `9586ac4dee53882ba4349710e94c950cf47fd8db` |
| 实际启动 | `steerline status --json`；维护目录 `status-after.json` | backend/Web ready，三个项目 ready 且 online，degraded=false |
| 页面访问 | `Invoke-WebRequest http://127.0.0.1:18742` | HTTP 200 |

Windows PowerShell 子进程在本次工具环境未能自动加载 `Get-FileHash`；改由当前 PowerShell 直接运行同一 `scripts/build-deploy.ps1` 完成构建部署。未更改机器设置。最后一包从当前工作区 `d22b93d8` 加本次目录校验修复构建，部署记录如实标记 dirty；其基准源码树与当时主目录提交 `21ec8ee6` 一致，未包含主目录并行编辑中的 takeover 文件。

未运行全量、L2/L3 或真实模型任务；本次验证仅覆盖目录接入、执行清理和用户授权的实际安装启动。新用例加入现有 `TestSteerRunnerUsesPublishedSchema`，由既有 L1 `quick-owner-runner-client` 收集。
