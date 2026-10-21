# 部署自动恢复中断宿主验收

范围与测试安排见 [Plan](../plans/deployment-interrupted-recovery.plan.md)。用户授权自动处理系统重启后的部署恢复，仍保留原身份、安装文件、配置/状态和候选接管核验。

| ID | 自动验收项 | 状态 / 证据 |
| --- | --- | --- |
| DIR-A01 | 精确退出的中断宿主可自动恢复原安装一次、等待 ready、正式停服；最终通过原 verify-restart 完整核验 | 受控通过；`app.log` 的缺失/陈旧回执正例、四项 readiness 分支；`receipt-fence.log` 补验陈旧回执 |
| DIR-A02 | 未知/活动身份、活动锁、非中断失败、安装文件/Runner pin/配置/checkpoint/项目集合不符、候选不兼容、恢复或最终核验失败均阻止替换；原 Run/输入与资源未核实事实保持 | 通过；`app.log` 的 22 项反例和既有只读/身份回归；`receipt-fence.log` 另验同实例但进程身份冲突；`candidate-binding.log` 核对原绑定保持 |
| DIR-A03 | PowerShell 仅在允许重启且首次核验失败时请求受控恢复一次；严格路径、成功路径和失败围栏不变，恢复证据随部署包保留 | 通过；Windows PowerShell 5.1 的 `powershell51.log`，覆盖同构建/换构建、恢复失败/缺证明/错候选及严格路径 |
| DIR-A04 | CLI 参数、构建、格式和脚本语法通过 | 通过；`cli.log`、`registration.log`、`parser.log`、`diff-check.log`；`go build -o .evidence/deployment-recovery-20261021/steerline.exe ./cmd/steerline` exit 0，gofmt 无输出 |

## 2026-10-21 执行记录

基线 `9e02854ef66e1b1e4b1fbdff35a9eeca294b26a5` 加本次未提交修改；Windows 本机，使用独立临时 home、Git/SQLite 与公开空闲 Runner CLI fixture。原始证据根目录为仓库 `.evidence/deployment-recovery-20261021/`。主应用定向回归 110.521s，收紧当前实例回执冲突后的补验 39.881s；CLI 4.531s，固定 L1 分区校验 1.458s，原候选绑定 3.086s，全部 exit 0。复现命令在 Plan；补验使用 `go test ./internal/app -run '^TestDeploymentInterruptedRecovery$/^(stale|matching-receipt|conflicting-receipt)$' -count=1 -v -timeout=120s`。

恢复正例在真实准入与最终核验之间，由受控生命周期回调提供新实例停机记录；readiness 通过认证的宿主/Web HTTP fixture 验证 starting、offline、failed、ready。实际原安装 EXE 的启动/正常停止接线复用既有生命周期入口，本轮没有实际运行安装版或制造电脑重启。原候选/Run/input/PR 记录按原字节比较，原生资源未知诊断保留；该结果不宣称真实原生结算成功。

初次编写夹具时，向已有测试 home 调用私有目录初始化导致 Windows 子项继承权限失效；改为仅原子写入 manifest 文件后补测通过。此问题仅发生在临时测试 home，未修改安装目录权限。

无人工阻塞项。未运行的实际安装、模型、full/L2/L3 不记通过。
