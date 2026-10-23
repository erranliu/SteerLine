# Spec：通用代理启动配置与 Orca 四档模型方案

用户直接授权安装目录两个 Orca 目标分别支持 gpt-6-luna high / xhigh，共四个真实执行配置，并确认补通用启动能力。不能仅改名称；旧 worker-start 不恢复。普通开发，不进入 roadmap。[Plan](../plans/agent-terminal-launch.plan.md) · [验收](../testing/acceptance/agent-terminal-launch.md)。

上层执行方案增加可选 agent_launch，冻结 executable、args、model、reasoning_effort。参数编译放在独立 agentlaunch 层；Orca 只接收不透明启动命令，不解析 Codex 参数、会话或日志。原生默认启动仍是独立能力，不是旧协议回退。

命令启动复用新工作区唯一默认终端，不创建第二个执行终端。准备工作区与启动代理是分别持久化的生命周期操作。启动命令仅发一次；保存原终端身份、得到原生输入接受后只读等待代理身份；原代理名称、runtime/terminal/incarnation/worktree/branch 核实后才发业务输入。未知发送不重发或另建会话。

本机、远端同一接口，远端仅增加固定 environment。命令语言明确为执行主机 PowerShell；宿主账号和目录信任由原生设置负责。权限参数由上层配置明确提供，安装四档保留现有 YOLO 等效参数，不改主机全局设置。业务输入、紧凑结果、Git/PR 和交付保持共用流程。

参数进入 profile 摘要、冻结快照、恢复和 UI 显示。已开始任务保持原冻结输入。安装只更新授权二进制、执行配置与说明；全部调试在独立包，保留三个 local Codex 配置和默认选择。

Codex 专用编译器增加会话级 tui.animations=false，并用无业务、无工具、无文件修改的 READY 初始化提示建立原生会话身份。这些均在上层生成，Orca 不读取或匹配其内容。业务投递仍要求原生身份及原生就绪确认；结果仍要求有效紧凑回复与原生 idle，通用等待窗口为五秒。配置原文与生成命令同时冻结并验证。
