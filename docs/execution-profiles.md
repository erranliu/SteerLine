# 全局执行配置

所有项目共享 `<STEERLINE_HOME>/execution.json` 中的执行方案。项目文件只配置仓库、任务与 PR 等项目属性；`host`、`agent`、`codex`、`orca`、`model`、`reasoning_effort`、`sandbox`、`windows_sandbox`、`profile`、`execution_profiles` 和 `default_execution_profile` 都只能写在全局文件的方案中。每个项目仍有独立的服务和 native Task DAG。

先将 [执行配置示例](../examples/execution.json) 复制到 `<STEERLINE_HOME>/execution.json`，再按机器环境修改。至少配置一套方案；有多套方案且启用自动派发时，须指定 `default_execution_profile`。只有一套方案时，它自动成为默认方案。完整项目文件见 [项目示例](../examples/config.json)。

```json
{
  "default_execution_profile": "local-strong",
  "execution_profiles": [
    {
      "id": "local-strong",
      "name": "本地 Codex 高强度",
      "host": "local",
      "agent": "codex",
      "model": "<明确模型>",
      "reasoning_effort": "xhigh",
      "sandbox": "workspace-write",
      "windows_sandbox": "elevated",
      "codex": {"executable": "<本机 Codex 可执行文件>"}
    }
  ]
}
```

按 `s` 打开全局方案列表，`↑/↓` 选择，`Enter` 确认，`Esc` 取消。服务在领取前核对方案 ID、摘要、Task 资格与目标能力；同一 Task 领取时冻结方案。编辑或删除全局方案不会重定向已开始的任务。控制台和服务加载的项目或全局文件内容不同时，控制台阻止新启动和自动派发；修改配置后重启服务并重新打开控制台。

Orca 本机方案可在同一全局文件中配置，五套 Luna 方案见[示例](../examples/execution.json)。使用 `host=orca`、`agent=codex`、`sandbox=workspace-write`，并填写 `orca.executable`、`orca.shell`、`orca.wait_timeout`。要固定模型与推理强度，`model` 和 `reasoning_effort` 必须一起填写，`codex.executable` 必须指向原生可执行文件，`orca.shell` 使用 `powershell.exe` 或 `pwsh.exe`。SteerLine 在 Orca 工作区中用固定命令启动该 Codex agent，启动意图与终端身份分别持久化；不支持在 Orca 方案中再叠加 Codex profile 或其他 sandbox。选择方案后，服务会检查 Orca 可连接且目标仓库已在本机 Orca 注册；检查失败时拒绝派发。远程方案使用 `host=orca_supervised`，由独立 `orca_remote_worker` 包处理远端协议。

## 远程 Orca worker

远程方案与本地 Codex、本地 Orca 共用 Task、Decision、Operation 和 PR 交付规则。Run 与协调 inbox 位于中心，工作区与 agent 位于指定远端。首次创建远端工作区和 worker，后续轮次复用同一终端与 incarnation；不创建本地替代执行。

在全局文件的 `execution_profiles` 中添加以下方案，替换连接、仓库与协调终端的真实标识：

```json
{
  "id": "orca-remote",
  "name": "远程 Orca Codex",
  "host": "orca_supervised",
  "agent": "codex",
  "model": "<明确模型>",
  "reasoning_effort": "high",
  "sandbox": "workspace-write",
  "orca": {
    "executable": "<本机 Orca 原生可执行文件>",
    "shell": "powershell",
    "wait_timeout": "2m",
    "supervised": {
      "connection_id": "<environment list 返回的 pairedDeviceId>",
      "environment_id": "<已保存的 environment ID>",
      "repository_selector": "id:<远端 repo ID>",
      "coordinator_terminal": "<中心已打开的 bootstrap terminal handle>",
      "poll_interval": "2s",
      "capability_report_version": 1,
      "remote_cancel_available": false
    }
  }
}
```

领取前检查两端 Orca 的版本族、连接身份、远端仓库的 Git remote 与所需 federation 能力，固定模型与推理强度。配置中的协调终端用于预检，每次 WorkflowRun 另建独立的中心协调终端。远端必须已具备 Codex、项目工具和 PR 发布权限。

原生 question 进入共享 Decision；活动 Dispatch 的反馈通过消息投递，已结算轮次的反馈通过原终端开启下一轮。输入回执丢失时只核对原 Task/Dispatch 或 question history，不重复启动或回答。中心重启保留原执行并重新核验，旧服务保存的未投递输入需要重新确认；断连或身份变化保留资源并显示未核实。远端路径按执行主机返回值保存，不按中心操作系统转换。

`worker_done` 只证明轮次结算。提供方证明对应 PR 已合并后才写 TaskDelivery、关闭 Task，并 release 已结算 worker 和关闭排空的中心协调终端；远端 worktree 保留。远程取消仍不可用，停止中心监督不会被显示为远端进程已经退出。

当前实现已通过隔离协议与 Service 测试。2026-09-30 保存的“台式机64g”连接不可达，本次未完成双机真机验收。

## Windows 本地 Codex 沙箱

Windows 本地受限方案将 `windows_sandbox` 固定为 `elevated`（省略时采用该默认值），并随执行方案摘要及 WorkflowRun 一起冻结。原生 app-server 用 `--config windows.sandbox="elevated"` 启动，不继承用户全局配置中的 `unelevated`。`workspace-write` 仍只开放原工作区和经过核验的 Git 元数据目录，不扩大为整个仓库目录或全权限。

显式 `unelevated` 只可用于本地 `read-only`；本地 `workspace-write` 使用 linked worktree 的分散可写根，unelevated 无法落实此策略，控制台标记方案不可用、领取入口提前拒绝。`codex.args` 不能重复覆盖受管的 Windows sandbox 值。非 Windows 启动不注入 Windows 参数；本次字段不改变 Orca 的启动行为，Orca 就绪流程仍按其独立 Spec 实施。

配置校验和成功创建 app-server 不证明实际工具已可执行。elevated 沙箱须在这台机器的原生 Codex 入口完成必要的系统设置；SteerLine 不自动配置 Windows 账户、ACL 或防火墙，不自动回退到 unelevated 或 danger-full-access。设置方式见[官方 Windows sandbox 文档](https://learn.chatgpt.com/docs/windows/windows-sandbox)。原有运行保持冻结权限，不通过编辑全局文件更换已有会话的沙箱方式。
