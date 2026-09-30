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

Orca 本机 Codex 方案使用 `host=orca`、`agent=codex`、`sandbox=workspace-write`，必须显式填写 `windows_sandbox`（`elevated` 或 `unelevated`）、原生 `codex.executable`、`orca.executable`、PowerShell `orca.shell` 和正的 `orca.wait_timeout`。示例选择 `unelevated`；选择方式由配置决定，故障后不会自动切换。model 和 reasoning_effort 可分别省略；省略时保留启动器默认解析语义。

有无显式 model 都使用同一受管命令，冻结 `approval_policy=never`、Windows sandbox、严格配置校验及 linked worktree 写入根目录。原始 `codex.args` 当前只允许独立的 `--no-alt-screen`、`--no-daemon`、`--search`；子命令、配置/Profile 注入、权限覆盖与目录扩张均在资源创建前拒绝。领取前的 CLI/Orca 查询不初始化沙箱；领取后在原 terminal 上运行 readiness 工具探针，核对真实工具事件、原子收据及原生成功结算后，才派发一次业务任务。

控制台显示冻结权限、环境核验及阻断来源。宿主等待时，文字保留原目标而不注入菜单；中心选项可打开原 terminal 或重新检查。重新检查先对账原操作，不用新会话替换丢失会话，不延长旧准备期限。运行中的 Execution 不受配置文件编辑影响。其他 agent 与 local 的执行/恢复语义保持原入口；受监管远端尚未接入。

本次已完成审计修复及隔离真机服务闭环；[验收记录](validation/worker-environment-readiness/README.md)区分真实工具证据、受控故障回归及已通过的真机服务闭环。

## Windows 本地 Codex 沙箱

Windows 本地受限方案将 `windows_sandbox` 固定为 `elevated`（省略时采用该默认值），并随执行方案摘要及 WorkflowRun 一起冻结。原生 app-server 用 `--config windows.sandbox="elevated"` 启动，不继承用户全局配置中的 `unelevated`。`workspace-write` 仍只开放原工作区和经过核验的 Git 元数据目录，不扩大为整个仓库目录或全权限。

显式 `unelevated` 只可用于本地 `read-only`；本地 `workspace-write` 使用 linked worktree 的分散可写根，unelevated 无法落实此策略，控制台标记方案不可用、领取入口提前拒绝。`codex.args` 不能重复覆盖受管的 Windows sandbox 值。非 Windows 启动不注入 Windows 参数；Orca 的受管启动和就绪流程见上文。

配置校验和成功创建 app-server 不证明实际工具已可执行。elevated 沙箱须在这台机器的原生 Codex 入口完成必要的系统设置；SteerLine 不自动配置 Windows 账户、ACL 或防火墙，不自动回退到 unelevated 或 danger-full-access。设置方式见[官方 Windows sandbox 文档](https://learn.chatgpt.com/docs/windows/windows-sandbox)。原有运行保持冻结权限，不通过编辑全局文件更换已有会话的沙箱方式。
