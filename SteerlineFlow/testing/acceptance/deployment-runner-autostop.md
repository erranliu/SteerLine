# Runner 更新自动停服与独立启动（2026-11-02）

用户安装更新在 `deploy.runner-upgrade-preflight` 因原 Runner 仍运行而失败：脚本把要求冷查询的排空校验放在自动停服之前。本次修复基于 `fea9df0692192f0236121299d8233e1e6f5f957f` 的工作树增量；不修改 Runner 协议、业务排空标准、提示词或数据库。

## 行为

- 完成构建、暂存、版本/pin 与执行配置校验后，通过现有单次 `prepare-deployment` 正式停服，再执行 `verify-runner-upgrade`，最后替换文件。Go 仍核验原服务退出、资源结算、全部 Run 业务终态和未完成控制；未排空就保留原 EXE/配置/数据库，不恢复或取消业务。失败信息明确服务可能已停止，不再将所有错误描述为旧 v8 状态问题。
- 新 `scripts/deployment-start.ps1` 通过本机 `Win32_Process.Create` 运行正式 `start`，避开自动化宿主的 kill-on-close Job；显式传入调用者环境、设置目标 home 并核对用户 SID。环境变量不写入磁盘或命令行，路径按 PowerShell 字符串字面量编码。
- 启动代理记录独立调用 ID、日志和原子发布的退出结果；父脚本检查本次结果并继续等待全部项目 ready。创建进程成功不作为启动成功，失败或未知结果不自动重发。`-NoStart` 保持不启动。

## 验证

证据根：`C:\SteerlineDebug\wt-df14-deploy-autostop\`。除停服脚本的终端输出外，下表路径相对此根；所有场景使用隔离假安装或受控测试，不更新真实安装环境。

| 验证 | 命令与证据 | 结果 |
| --- | --- | --- |
| PowerShell 5.1 部署连接 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-build-deploy-pipeline.ps1 -OutDir <证据根>\diagnostics\pipeline-20261102-01`；`result.json` 及各场景 `output.log/timings.json` | 11 项通过：首次预检失败、首次安装、固定 Runner 复用、实际改变 SHA 的 Runner 更新、NoStart、构建/停服/启动/ready/排空/已有安装预检失败。核实先 stop 再排空再 install；失败不替换、不启动，EXE 和执行配置摘要不变。启动假 EXE 经实际 Windows 代理执行，公开 Win32 查询确认无 kill-on-close Job，并消费显式传入的测试环境。home 含空格、单引号、`&` 和模板占位符，完整路径场景通过。 |
| 原停服与证据绑定回归 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1`；终端 exit 0 | 单次 Go 准备调用、调用/包/候选绑定、固定 Runner 复用与升级失败门禁通过。 |
| 固定 L1 | `go run ./cmd/test-feedback -level=L1 -out <证据根>\test-results\L1\20261102-01`；`summary.json` | passed=true；准备 0.35s，执行 17.69s。 |
| 固定 L2 | `go run ./cmd/test-feedback -level=L2 -out <证据根>\test-results\L2\20261102-01`；`summary.json`、`scenarios/smoke.delivery/scenario.json` | passed=true；准备 12.47s，执行 118.18s，cleanup_complete=true，cleanup_errors=null。 |
| 文档与差异 | `python scripts/check_doc_links.py`、`git diff --check` | 通过。 |

本轮不执行真实模型/远端 PR，不将前一阶段用户授权的实际安装维护算作新脚本部署验收。Windows 进程管理接口不可用、启动用户不符或业务未排空时仍明确失败；不增加兼容、迁移、清库或绕过生命周期检查的路径。
