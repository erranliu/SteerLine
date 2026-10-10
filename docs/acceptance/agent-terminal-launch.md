# 通用代理启动与四档配置验收

只归属本次直接授权的 [Spec](../spec/agent-terminal-launch.spec.md) / [Plan](../spec/agent-terminal-launch.plan.md)，不进入 roadmap。

| ID | 自动项 | 结果 |
| --- | --- | --- |
| ATL-A1 | 参数、显示、冻结与错误拒绝 | 通过；agent-launch-frozen-command-tests.log、agent-launch-final-audit-tests.log |
| ATL-A2 | 一终端一次启动、身份、未知效果与恢复 | 通过；受控本机/远端三阶段与恢复测试；原始身份、未知效果不重发单元测试 |
| ATL-A3 | 本机/远端 high/xhigh 实际启动与结果 | 通过；四档原生显示和 READY，两个目标真实完整交付见下 |
| ATL-A4 | 安装四档，保留默认/local/任务事实 | 通过；C:/SteerlineDebug/agent-launch-deploy/，四档解析、冻结、命令及摘要检查；服务 connected/ready |

初步包 C:/SteerlineDebug/dev-20261010-190156-3ed260c4：terminal create --command 两次未取得有效创建句柄，保留失败，不作为产品实现。复用唯一原 shell 输入启动命令后，原生显示 GPT-6-Luna high，返回 nonce 的正确结果；agentIdentity 后续为 codex，incarnation 与原启动请求一致。回执在 evidence/launch-probe2-*；这不是最终产品验收。

最终代码真实验收（2026-10-10）：

- 本机 high：C:/SteerlineDebug/dev-20261010-204229-6d4b4fa2-dirty/evidence/，原终端 term_59091604-57c7-4920-9089-6fefc706a96a，三阶段均成功；[测试 PR #210](https://github.com/erranliu/SteerlineTest/pull/210) 合并到包专属 base，TaskDelivery 已登记，任务 1 closed/completed。
- 远端 xhigh：C:/SteerlineDebug/dev-20261010-204507-6d4b4fa2-dirty/evidence/，原终端 term_175185d8-01be-4d6e-8c16-0b0082ab7668；[测试 PR #211](https://github.com/erranliu/SteerlineTest/pull/211) 合并到包专属 base，TaskDelivery 已登记，任务 1 closed/completed。
- 本机 xhigh：本机包 xhigh-probe-{create,terminals,start,idle,screen}.json，原终端 term_84e36df4-aeb5-47dd-adac-9f94701440cd；实际 GPT-6-Luna xhigh、READY 与原生 idle=true。
- 远端 high：远端包 high-probe-{create,terminals,start,idle,screen}.json，原终端 term_ffe0174b-c044-44d3-ba3f-d98a8a04d581；实际 GPT-6-Luna high、READY 与原生 idle=true。

真实使用两端 Codex 0.162.1。安装目录没有用于测试。high/xhigh 各目标使用相同传输流程；另外两档只验启动与响应，未重复完整交付。初始化时自建目录的原生信任菜单确认一次，不补业务 Enter、不重发任务。

历史失败保留：194515 包模型提前发布 #209（已关闭未合并），且一秒 idle 不能收敛；194738 包结果正确但同样 idle 超时。201556 包创建后初始身份绑定未核实，不发业务输入，未重建资源。最终代码使用会话级禁动画、五秒原生 idle 和 READY 初始化；204229/204507 成功，不将历史失败改为通过。日志均在 C:/SteerlineDebug。

已安装二进制为 8e3e88d8（source_dirty=false），SHA-256 EF1F62A638C2CD01C784CFA4B0534BCC495E3EA700C35907CAD28BCCF1CF3B38。C:/SteerlineRuntime/execution.json 共七项，三个 local 和默认原样，四个 Orca 使用实际参数。安装前后五个任务的身份/状态/版本及三个冻结执行方案逐项相等；未重置任务。公共 stop-service 正常停止，按原手动服务窗口重开，并重载控制台；服务身份/config 匹配、ready=true。原文件备份 C:/SteerlineRuntime/backup/orca-agent-launch-20261010-2108/。自建探针终端及调试服务全部关闭，句柄检查及关闭回执 C:/SteerlineDebug/cleanup-term_*-{show,close}.json；关闭 PTY 不宣称所有外部写者已消失。
