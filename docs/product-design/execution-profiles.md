# 全局执行配置

第一阶段包含本地 Codex、本机 Orca 与远端路径交付。多后端和多候选保留；远端缺证阻断保持，不以本机验证推断远端支持。

所有项目共享 `<STEERLINE_HOME>/execution.json`，全局文件格式2，每个方案有独立 host、agent 和实现 codec；模型配置属于支持它的实现。多套执行方案与默认方案选择保留。项目负责仓库、Git/PR provider、任务与publication_policy，不再提供权限模式或附加需求。

复制[全局示例](../../examples/execution.json)到execution.json，按机器环境修改；项目文件见[项目示例](../../examples/config.json)。Codex profile codec格式2只接受executable、model、reasoning_effort。旧args/profile/windows_sandbox及审批覆盖即使为空也拒绝；Git/PR executable/argv与Orca连接设置继续保留。local的effort省略时仍为high；Orca 不接受 model/effort 字段，模型与权限由执行主机的原生启动器管理。

```json
{
  "version": 2,
  "default_execution_profile": "local-luna",
  "execution_profiles": [{
    "id": "local-luna", "name": "本机 Codex", "host": "local", "agent": "codex",
    "implementation_configs": [{
      "implementation_key": "codex.worker-profile", "codec_version": 2,
      "payload": {"executable": "codex.exe", "model": "gpt-6-luna", "reasoning_effort": "medium"}
    }]
  }]
}
```

## Windows 本地 Codex 沙箱

Windows local Codex采用唯一原生方案：workspace-write、windows.sandbox=unelevated、approval_policy=on-request、approvals_reviewer=auto_review、工具网络allow。adapter生成这些设置；全局或项目不能覆盖。local Codex受管命令使用独立原生进程与同一设置，首轮、反馈和恢复保持原绑定。

候选工作区及必要Git对象/refs/logs/自身worktree admin根由现有资源准备一次绑定；隐式TMP/TEMP及slash tmp写入仍排除，不授权项目总目录、共同Git总目录或兄弟候选目录。普通操作在原生沙箱内执行；需要批准时交给原生自动reviewer。审核拒绝、不可用或启动失败沿原反馈/环境阻断报告原因，不增加SteerLine审批器、人工审批队列、自动修复、elevated或full-access回退。自动审核不保证全部放行，也不能批准Windows UAC。

SteerLine不做管理员安装、系统账号/防火墙设置、全局配置改写、ACL修复或通用缓存发现。依据[原生自动审核](https://learn.chatgpt.com/docs/sandboxing/auto-review)与[Windows沙箱](https://learn.chatgpt.com/docs/windows/windows-sandbox)；本机验收见[第二批记录](../acceptance/archive/completed/product-simplification-02-configuration/README.md)。其他agent、非Windows和远端不套用不适用的参数，沿原adapter支持范围及目标/回执检查。

## Orca 单终端：本机与远端

统一使用 `host: orca`、`orca.terminal-profile` 格式 2 和 `orca_terminal_v3`。agent 是 Orca 注册的原生启动器标识，不由 SteerLine 拼接模型命令。payload 显式指定 CLI、固定 repository_selector 和 wait_timeout；environment 省略表示本机，指定后所有 worktree / terminal 调用都路由到该环境。

本机与远端都只有一个受管 agent 终端。工作区创建时不传 prompt；上层 worker 保存短输入和紧凑结果契约，下层通过 terminal send 投递一次、通过原游标 terminal read 读取回复。accepted 与 turn_started 分开记录。原 runtime / terminal / incarnation / worktree / branch 不符、输出裁剪、缺失结果或冲突结果，均不证明完成，也不改投其他终端或补 Enter。

不提供 Orca agent 专用日志、模型覆盖、完整会话全文读取、原生提问路由或强制报告只读权限。两端仅通过 worktree show 比较提交与分支；inspection_scope=revision 不代表未提交工作区已经验证干净。业务结果仍经同一 development/v2 与 Git/PR 权威检查，发布 JSON 必须含 PR URL，必要时进入一次 publish.report。停止只关闭原终端，不把 PTY 关闭扩大为所有子进程写入者已消失。

```json
{
  "executable": "orca.exe",
  "environment": "paired-environment-id",
  "repository_selector": "id:fixed-repository-id",
  "wait_timeout": "30s"
}
```

旧 host=orca_supervised、对应 codec / Run / Task / Dispatch 生命周期与旧 Orca codec 直接删除。没有迁移、兼容读取或回退。当前 [Spec](../spec/orca-terminal-transport.spec.md)、[Plan](../spec/orca-terminal-transport.plan.md) 与[验收](../acceptance/orca-terminal-transport.md)。
