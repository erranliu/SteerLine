# SteerLine 执行环境要求与验收

环境检测由既有 Runner 目标 Agent 调用本项目入口，不由脚本再启动 Agent。环境适配遵循[项目环境适配规范](spec/project-environment-adaptation.spec.md)；业务测试入口及通过标准仍见[测试指南](testing.md)。

```steerline-environment
{"version":1,"script":"scripts/verify-environment.ps1","interpreter":"powershell.exe","validation_base":"codex/environment-validation/steerline","dependencies":["scripts/environment-tty.ps1","go.mod","go.sum","SteerlineFlow/testing/l1.md","SteerlineFlow/testing/l2.md","tests/e2e/suites.json","cmd/test-feedback/managed.go"]}
```

```steerline-pr-acceptance
{"required_checks":[],"minimum_approvals":0,"l2_summary":".steerrunner/artifacts/environment-l2/summary.json"}
```

## 用户预先准备

| 必需项 | 约定入口与验证 |
| --- | --- |
| TTY | 同一目标 Agent 的 `exec_command(tty:true)`；实际命令启动、stdin/stdout均未重定向、输出`STEERLINE_TTY_OK`、退出0 |
| PowerShell | Windows 自带的 `powershell.exe`（5.1）；检测脚本不依赖 PowerShell 7 |
| Git | PATH中的`git`，既有仓库认证及用户提交身份；不交互登录或修改Git配置 |
| Go | PATH中的`go`，版本至少1.26.0，模块依赖已准备好 |
| GitHub | PATH中的`gh`，既有登录能访问erranliu/SteerLine及完成专属验证base的PR；不复制凭据 |
| 源码 | Runner分配的独立分支，干净且HEAD与本轮固定source_commit一致 |

用户准备执行账号的工具、PATH和认证；Agent不搜索安装目录、不安装/修复依赖或修改环境变量。分支及仓库规则由外部L4驱动通过provider核实；Agent不查询保护策略，不能猜测验证base继承规则或绕过检查。

## Agent执行顺序

先以真实`exec_command(tty:true)`执行：

```powershell
powershell.exe -NoProfile -File scripts/verify-environment.ps1 -Phase tty
```

最小入口只检查当前进程终端、输出JSON和标记，不执行测试或PR。必须核对实际工具启动、输出及退出状态；普通shell或Orca能打开不算通过。工具未启动、失败、超时或未验证均阻断，不允许切换`tty:false`替代或手写检测成功。

随后仍通过`exec_command(tty:true)`执行同一入口的`-Phase prepare`。脚本再次现场检测TTY并记录`checks`中的`terminal.tty`，所有独立环境项检查完成；未全部满足则原子写失败报告，不进入L1/L2。满足后只转调原`go run ./cmd/test-feedback -level=L1`与完整`-level=L2`，使用本轮新输出目录，核对原managed报告、summary字节摘要、固定源码及清理，不运行Full或额外L3/L4。

仅prepare完整通过才执行`-Phase publish`，同样要求TTY。它在原Runner head提交专属验证文档，完成真实push、唯一PR创建/查询及身份核验，目标只能是文档声明的专属验证base，禁止main/master。外部写入前保存未知状态；未知操作只查询原分支/原PR，不能重新执行写入。脚本不删除Runner持有的分支或工作区。

提交前，固定脚本核验暂存diff非空、暂存区仅含本轮探针且正文与本轮activity/round/source一致，并执行`git diff --cached --check`；输出原暂存diff后才commit。任何核验失败均不提交。

## 材料固定与未就绪条件

固定验证base必须由项目维护者通过正常Git流程预先创建并更新到包含当前正式源码；脚本及环境活动不会创建、恢复或删除它。本次材料PR不执行验证base创建或合并；缺少base保持not_ready。环境执行规则按同版本SteerLine规范保存为`SteerlineFlow/environment-execution.md`后才能适配。

prepare/publish只读取Runner注入的`STEERLINE_PROGRESS_FILE`和`STEERLINE_RESULT_FILE`，保留原活动、轮次、源码和脚本摘要，原子写结果。TTY成功只是一项必需条件，不替代完整L2、真实PR、原Run收尾及结果封存；缺报告、未停止或清理未核实均不能ready。文档及脚本存在不表示当前机器已经通过环境验收。

## TTY前置失败只记录

TTY工具明确未启动，或纯最小探针已结束且明确失败时，保留原工具错误，不换封装重跑检测。若原Agent仍有可用工具，可仅为记录失败使用普通工具（允许tty:false或其他shell）调用：

```powershell
powershell.exe -NoProfile -File scripts/verify-environment.ps1 -Phase tty-failure -FailureReason '原工具错误，不含凭据'
```

此入口不运行TTY探针、prepare、测试、认证或PR，只用原子写入记录terminal.tty=failed、缺失项incomplete，保留原活动/轮次/源码/脚本摘要、previous及同轮已有事实；不得新增passed。入口退出1表示负结果已写，核对result的completed=true和原身份后返回blocked，不因退出1重跑。completed只表示报告完成，环境仍not_ready。

入口只接受初始空报告，或同轮已完成且脚本自有清理已确认的报告；PR URL/head_revision、发布未知、PR进度true、部分报告或非法输入均拒绝，不能清除已有发布事实。超时/受理未知、prepare可能仍有生产者或测试进程时不使用此入口，沿原停止/对账处理。全部工具不可用则保留原未完成报告，不手写result。正常tty/prepare/publish仍只能通过真实TTY执行。


## 发布验收边界

环境报告version=2。目标Agent只验证工具、TTY、完整L2、专属提交/push和原PR创建/查询，记录原URL、base/head及head_revision。分支策略核实、必要检查/审核、精确PR合并及远端测试资源清理由外部交付或L4驱动承担；Agent不查询保护策略或合并PR，不把未执行动作写成通过。ready表示执行及发布能力通过；外部L4闭环结果独立记录，不回写已封存result。
