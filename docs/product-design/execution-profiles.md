# 全局执行配置

当前部署方案为本机 Codex 和远端 Orca，暂时不提供本机 Orca 可选方案。多后端和多候选保留；远端缺证阻断保持，不以本机验证推断远端支持。

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

## 远端 Orca 单终端

部署配置中的 Orca 方案只使用远端 environment，示例目标为 `台式机64G`；本机任务使用 Codex 方案，不需要把本机登记为 Orca 远程主机。完整 v2 runtime pair code 通过 `orca environment add --name <名称> --pairing-code <完整链接> --json` 保存到用户的 `orca-environments.json`；执行方案只引用 environment ID 或 `name:<名称>`，不保存 token，也不依赖全局配对环境变量。增加远端时新增 environment 和对应方案，各实例认证互不覆盖。调试模板默认使用本机 Codex，`scripts/new-debug-release.ps1 -Orca` 默认使用 `name:台式机64G`，其他远端通过 `-OrcaEnvironment` 显式选择。就绪超时对原终端最多复查一次；明确交互阻塞保留诊断且不发送输入。严格绑定的拒绝回执保留 rejected，投递未知仍不得重发。

保存连接配置及两端认证访问的验证见[配置验收](../acceptance/orca-pair-configuration.md)。

统一使用 `host: orca`、`orca.terminal-profile` 格式 3 和 `orca_terminal_v3`。agent 是 Orca 注册的原生启动器标识，不由 SteerLine 拼接模型命令。payload 仅指定 CLI、wait_timeout 和可选 environment，不再指定仓库目录或 ID。同一执行方案可跨项目复用：没有 environment 时直接使用当前项目的仓库路径；指定 environment 时，先列出该环境中已注册的 Git 仓库，按实际路径的最后一级目录名与当前项目仓库匹配，再用唯一匹配的仓库 ID 创建工作区。匹配不使用 Orca 显示名称；没有匹配、重名或列表不完整时在创建前明确拒绝，不选择其他项目。恢复只使用原工作区创建回执，不重新按目录名匹配。

每个远端执行只有一个受管 agent 终端。工作区创建时不传 prompt；上层 worker 保存与 Codex 一致的阶段提示词，不添加 SL_RESULT、base64 或 nonce 指令；下层通过 terminal send 原样投递一次，通过原请求回执、原生主代理状态及原终端完整可见回复核验本轮普通结果。accepted 与 turn_started 分开记录。主代理结束由订阅/补查询确认，可见完成行不作为执行结束信号；本轮普通原文与完整可见回复一致后才推进。配对订阅按固定 environment 读取 Orca 保存的认证。原 runtime / terminal / incarnation / worktree / branch 不符、输出裁剪、缺失输入/回复边界或未确认开始，均不证明完成，也不改投其他终端或补 Enter。

不提供 Orca agent 专用日志、模型覆盖、完整会话全文读取、原生提问路由或强制报告只读权限。执行端仅通过 worktree show 比较提交与分支；inspection_scope=revision 不代表未提交工作区已经验证干净。业务结果仍经同一 development/v2 与 Git/PR 权威检查，发布 JSON 必须含 PR URL，必要时进入一次 publish.report。停止只关闭原终端，不把 PTY 关闭扩大为所有子进程写入者已消失。

```json
{
  "executable": "orca.exe",
  "environment": "paired-environment-id",
  "wait_timeout": "30s"
}
```

旧 host=orca_supervised、对应 codec / Run / Task / Dispatch 生命周期与旧 Orca codec 直接删除。没有迁移、兼容读取或回退。当前 [Spec](../spec/orca-terminal-transport.spec.md)、[Plan](../spec/orca-terminal-transport.plan.md) 与[验收](../acceptance/orca-terminal-transport.md)。

2026-10-14 仓库自动选择的实现与验证见[专属验收](../acceptance/orca-project-repository.md)。格式 2 与旧 repository_selector 不再接收，无迁移或兼容读取。

已有安装数据库若冻结了旧格式执行方案，服务会拒绝启动；只改 execution.json 不足以升级。安装更新须先明确处理旧执行状态，不自动改写已冻结方案，也不清除任务/执行记录。没有历史运行状态的新环境直接使用格式 3。
