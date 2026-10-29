# 验收：项目环境必需TTY与固定入口

2026-10-29。对应[环境适配Spec](../../spec/project-environment-adaptation.spec.md)、[Plan](../../plans/project-environment-adaptation.plan.md)和[本项目环境要求](../../environment.md)。本次仅补材料及环境准入，不改Runner原生输入、启动确认、权限、workflow或业务测试通过标准。

## 实施与验证范围

环境材料生成和执行规范明确：既有Runner远端Agent是编排者，必须用真实`exec_command(tty:true)`调用项目入口；脚本检查实际stdin/stdout、写`terminal.tty`，不启动嵌套Agent。prepare/publish现场验证；最小tty阶段不跑L1/L2或PR。环境报告即使L2/PR全绿，没有通过的TTY项也不得ready。

新增PowerShell入口复用本项目原固定L1/L2，核对原managed报告、summary字节/摘要、原源码和清理。发布沿现有gh和Git入口，仅允许专属验证base；未知写入保持未知，不降级重试。环境执行及生成规则作为同版本文件保存，短提示正文不增加内联材料。

定向脚本夹具在临时Git及局部工具桩中运行，核对重定向拒绝、旧TTY通过不能替代当前检测、完整L2/发布零调用、原元数据保留及进度不误置true；它不证明真实远端TTY成功。测试命令为`python scripts/test-environment-tty.py`。Go定向测试覆盖环境领域、提示词和应用接线；固定L1及受控L2按项目指南执行，实际结果在提交前补记。

## 未验与前置

未启动真实Agent、Unity、完整EditMode、业务全量测试或真实环境PR；未安装、合并或部署。TTY正向、完整远端L2+真实PR及两轮原测试进度复验均未因此通过。

项目维护者须预先准备文档声明的专属验证base并更新到包含合入后的正式源码；本次不创建验证base、不修改机器环境或旧现场。材料存在及受控测试通过不能把环境标记ready。执行规则中的报告仅是同权限脚本的声明，不能声称防伪或替代原工具证据。

初次脚本夹具在桌面沙箱临时目录创建子目录时遭遇访问拒绝，未运行测试或发布；普通授权的隔离验证随后通过，失败如实保留，不当成TTY产品失败。原始日志在本次仓库外独立验证目录，旧业务证据保持不变。

## 本次结果

- 环境领域、提示词及应用接线定向回归通过；新增TTY准入三种负向及通过项测试通过。
- SteerLine与308入口各两项隔离脚本测试通过；PowerShell语法解析通过。
- 固定L1通过；受控L2通过且cleanup_complete=true，不含真实模型或provider发布。
- Markdown导航607份文件、3854条本地链接、0错误；git diff --check通过。
- 原始输出：`C:/SteerlineDebug/wt-507e-environment-tty-20261029/`，含owners-01.log、tty-admission-02.log、两个项目脚本日志及L1-01/L2-01原summary。以上结果不升级远端环境、TTY正向或原业务两轮复验。
