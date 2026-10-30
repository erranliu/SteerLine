你负责为当前项目生成或维护两份配套的Git文件：SteerlineFlow/environment.md 和 scripts/verify-environment.ps1（pwsh）或 scripts/verify-environment.sh（bash）。只生成内容，不执行、提交或合并。

验证目标：目标环境完整L2和真实PR，复用正式清单及入口，不选Task子集、不要求全L3/Full、不跳过或模拟必需环节。版本默认最低兼容，不把CI示例变成精确门禁；仅明确的引擎、资源格式、锁定依赖约束固定版本。更高版本仍须通过原适配器与真实测试。缺源码、依赖、凭据引用或PR规则写gaps，不编造已验证能力或无条件通过脚本。

已有准备说明、测试入口和PR工具时，environment.md只做固定元数据、引用及缺口补充，verify-environment只薄调用原程序、核验原报告并转换双文件结果，不复制实现或另建测试/认证框架。沿用已有Orca配对和仓库remote身份，不要求本机与远端路径相同。普通任务的PR发布规则保持原意；环境验收的合并授权只作用于专属验证base，文档须明确这一差异。

TTY是必需环境能力。环境验证由Runner启动的同一目标Agent编排，不由脚本再启动Agent。文档必须要求Agent通过真实exec_command(tty:true)调用检测入口，核实启动成功、输出STEERLINE_TTY_OK和退出0；脚本检查实际进程stdin/stdout均未重定向，并在checks写terminal.tty。普通shell、Orca终端可打开、tty:false成功和模型自述均不能替代；失败、超时、工具未启动或无法核实保持failed/incomplete并阻断完整L2与PR。提供不跑业务测试的最小TTY入口；prepare及publish仍现场检测，不复用其他Run或旧探针。辅助脚本列入dependencies，执行规则、文档和脚本须配套维护；不得安装修复、自动降级或手写通过。

environment.md 必须有且仅有一个 ```steerline-environment JSON代码段：{"version":1,"script":"scripts/verify-environment.ps1","interpreter":"pwsh","validation_base":"项目已预先存在的验证分支","dependencies":["项目依赖锁文件等有限真实相对路径"]}。bash脚本相应更换路径和解释器。文档写清工具、服务、非秘密配置/凭据引用、持久准备与新工作区初始化、命令、故障和人工处理。验证base由项目正常Git流程预先维护，不能是业务主分支；不得自动建删base。

项目文档声明要求，用户在目标环境预先安装工具并配置环境变量/PATH，实际执行进程必须能读取。Agent只执行固定脚本验收，不搜索安装目录/注册表，不安装、修复依赖或设置环境变量/Git配置，不要求用户现场提供路径。工具入口只能使用文档声明的变量或PATH命令；不提供固定安装路径或自动发现回退。脚本逐项检查所有独立要求，一项失败仍继续其余检查；缺少依赖而无法核实的项标incomplete并解释，不冒充通过。

同一个脚本接受prepare/publish阶段。prepare先检测环境要求，全部满足后才验证完整L2，不创建PR；失败原子写最终报告。publish仅在prepare完整通过时，使用Runner固定publication base和git branch --show-current所得head，提交专属验证改动、push、创建唯一PR、完成必需检查/审核、实际合并、核实合并和本次脚本资源清理。未知操作先查询原PR，禁止重新创建。禁止Agent修改受测实现/验收标准或手写通过报告；不删除Runner仍持有的工作区/分支和预期安装成果。

脚本摘要针对固定提交的原始Git blob字节，不能以Windows工作区CRLF转换判为材料变更。缺失版本化文件或LFS内容时报告未满足，不能自行恢复、改源码标准、借其他活动结果或修改注册仓库的业务checkout。

文档用一个 ```steerline-pr-acceptance JSON代码段记录真实PR规则和完整L2原始summary相对路径：{"required_checks":["项目必需检查名"],"minimum_approvals":0,"l2_summary":".steerrunner/artifacts/environment-l2/summary.json"}。示例值必须按项目实际规则替换，无必需检查才用空数组；不能假定验证base继承业务分支保护。完整L2原入口须把原报告写到该固定、Git忽略的相对路径，保留项目原格式和全部明细，以供真实L4独立核验；不可用包装器自述通过代替原报告。无法获知规则时写gaps，不猜测为零。

路径仅取环境变量：STEERLINE_PROGRESS_FILE更新已声明的test.<id>及pr布尔进度；STEERLINE_RESULT_FILE写最终JSON，先读取并保留SteerLine初始化的关联字段。均用同目录临时文件、完整刷新、原子替换；测试入口不得并发覆盖最终报告。报告最多64KiB，完整日志留远端，失败原因真实精简、不含凭据。

脚本必须在返回前写完最终报告。Runner只在原执行结束且生产者停止后封存一次result，运行中不回传报告；不要等待回传才结束，不要通过resume或修改已封存文件刷新结果。文件采集失败由Runner单独报告，不得据此伪造业务通过。

最终报告字段和语义以同版本 SteerlineFlow/environment-execution.md 的报告契约为准，生成文档和脚本前须完整读取并保留。

仅按输出schema返回完整文档、脚本及缺项。已有材料适用则复用，不扩展语言框架、目录扫描、认证系统或测试平台。
