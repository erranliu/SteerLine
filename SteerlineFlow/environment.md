# SteerLine 执行环境要求与验收

环境检测由既有 Runner 目标 Agent 调用本项目入口，不由脚本再启动 Agent。环境适配遵循[项目环境适配规范](spec/project-environment-adaptation.spec.md)；业务测试入口及通过标准仍见[测试指南](testing.md)。

```steerline-environment
{"version":1,"script":"scripts/verify-environment.ps1","interpreter":"pwsh","validation_base":"codex/environment-validation/steerline","dependencies":["scripts/environment-tty.ps1","go.mod","go.sum","SteerlineFlow/testing/l1.md","SteerlineFlow/testing/l2.md","tests/e2e/suites.json","cmd/test-feedback/managed.go"]}
```

```steerline-pr-acceptance
{"required_checks":[],"minimum_approvals":0,"l2_summary":".steerrunner/artifacts/environment-l2/summary.json"}
```

## 用户预先准备

| 必需项 | 约定入口与验证 |
| --- | --- |
| TTY | 同一目标 Agent 的 `exec_command(tty:true)`；实际命令启动、stdin/stdout均未重定向、输出`STEERLINE_TTY_OK`、退出0 |
| PowerShell | PATH中的`pwsh` 7或更高版本 |
| Git | PATH中的`git`，既有仓库认证及用户提交身份；不交互登录或修改Git配置 |
| Go | PATH中的`go`，版本至少1.26.0，模块依赖已准备好 |
| GitHub | PATH中的`gh`，既有登录能访问erranliu/SteerLine及完成专属验证base的PR；不复制凭据 |
| 源码 | Runner分配的独立分支，干净且HEAD与本轮固定source_commit一致 |

用户准备执行账号的工具、PATH和认证；Agent不搜索安装目录、不安装/修复依赖或修改环境变量。主分支及仓库规则须通过provider查询；现有公开main未保护且无ruleset，脚本发现规则变化则阻断，不能猜测验证base继承规则或绕过检查。

## Agent执行顺序

先以真实`exec_command(tty:true)`执行：

```powershell
pwsh -NoProfile -File scripts/verify-environment.ps1 -Phase tty
```

最小入口只检查当前进程终端、输出JSON和标记，不执行测试或PR。必须核对实际工具启动、输出及退出状态；普通shell或Orca能打开不算通过。工具未启动、失败、超时或未验证均阻断，不允许切换`tty:false`替代或手写检测成功。

随后仍通过`exec_command(tty:true)`执行同一入口的`-Phase prepare`。脚本再次现场检测TTY并记录`checks`中的`terminal.tty`，所有独立环境项检查完成；未全部满足则原子写失败报告，不进入L1/L2。满足后只转调原`go run ./cmd/test-feedback -level=L1`与完整`-level=L2`，使用本轮新输出目录，核对原managed报告、summary字节摘要、固定源码及清理，不运行Full或额外L3/L4。

仅prepare完整通过才执行`-Phase publish`，同样要求TTY。它在原Runner head提交专属验证文档，完成真实push/唯一PR/正常合并及核验，目标只能是文档声明的专属验证base，禁止main/master。外部写入前保存未知状态；未知操作只查询原分支/原PR，不能重新执行写入。脚本不删除Runner持有的分支或工作区。

## 材料固定与未就绪条件

固定验证base必须由项目维护者通过正常Git流程预先创建并更新到包含当前正式源码；脚本及环境活动不会创建、恢复或删除它。本次材料PR不执行验证base创建或合并；缺少base保持not_ready。环境执行规则按同版本SteerLine规范保存为`SteerlineFlow/environment-execution.md`后才能适配。

prepare/publish只读取Runner注入的`STEERLINE_PROGRESS_FILE`和`STEERLINE_RESULT_FILE`，保留原活动、轮次、源码和脚本摘要，原子写结果。TTY成功只是一项必需条件，不替代完整L2、真实PR、原Run收尾及结果封存；缺报告、未停止或清理未核实均不能ready。文档及脚本存在不表示当前机器已经通过环境验收。
