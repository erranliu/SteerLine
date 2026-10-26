你负责为当前项目生成或维护两份配套的Git文件：SteerlineFlow/environment.md 和 scripts/verify-environment.ps1（pwsh）或 scripts/verify-environment.sh（bash）。只生成内容，不执行、提交或合并。

目标是远端环境能执行项目完整L2及完整真实PR。复用提供的正式测试清单和入口；不选择Task子集、不要求全L3/Full，不跳过/模拟必需环节。缺少源码、依赖、凭据引用或PR规则时写gaps，不发明已验证的能力或无条件通过脚本。

environment.md 必须有且仅有一个 ```steerline-environment JSON代码段：{"version":1,"script":"scripts/verify-environment.ps1","interpreter":"pwsh","validation_base":"项目已预先存在的验证分支","dependencies":["项目依赖锁文件等有限真实相对路径"]}。bash脚本相应更换路径和解释器。文档写清工具、服务、非秘密配置/凭据引用、持久准备与新工作区初始化、命令、故障和人工处理。验证base由项目正常Git流程预先维护，不能是业务主分支；不得自动建删base。

同一个脚本接受prepare/publish阶段。prepare只准备和验证完整L2，不创建PR；失败原子写最终报告。publish仅在prepare完整通过时，使用Runner固定publication base和git branch --show-current所得head，提交专属验证改动、push、创建唯一PR、完成必需检查/审核、实际合并、核实合并和本次脚本资源清理。未知操作先查询原PR，禁止重新创建。禁止Agent修改受测实现/验收标准或手写通过报告；不删除Runner仍持有的工作区/分支和预期安装成果。

两个文件路径只读环境变量：STEERLINE_PROGRESS_FILE供布尔进度（更新声明过的test.<id>及pr）；STEERLINE_RESULT_FILE供最终JSON。先读取结果初始JSON保留本轮关联字段，初始化内容由SteerLine提供。每份文件采用同目录临时文件+完整刷新+原子替换；不要让被调用测试入口并发覆盖最终报告。报告最多64KiB，完整日志留在远端，输出精简真实失败原因且不含凭据。

脚本必须在返回前写完最终报告。Runner只在原执行结束且生产者停止后封存一次result，运行中不回传报告；不要等待回传才结束，不要通过resume或修改已封存文件刷新结果。文件采集失败由Runner单独报告，不得据此伪造业务通过。

最终报告保持结构：{"version":1,"activity_id":"原值","round":1,"source_commit":"原值","script_digest":"原值","completed":true,"l2":[{"test_id":"正式ID","status":"passed|failed|incomplete","message":"真实摘要"}],"pr":{"url":"","base":"固定base","head":"实际分支","checks_passed":false,"reviews_passed":false,"merged":false,"merge_commit":"","cleanup_completed":false,"outcome_unknown":false},"message":"失败或需人工处理的原因"}。completed仅表示报告结束，非成功。必须列出全部L2，不以零匹配/跳过/超时为通过；L2失败时PR保留未执行，不伪造通过。PR写入/合并未知时outcome_unknown=true；已创建资源未完成清理时如实写false。

仅返回匹配约定输出schema的JSON：document为完整准备文档文本，script为完整脚本文本，gaps为缺项字符串数组。已有材料仍适用时保持简洁，不扩展脚本语言框架、目录扫描、认证系统或测试平台。
