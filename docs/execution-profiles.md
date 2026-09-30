# 全局执行配置

所有项目共享 `<STEERLINE_HOME>/execution.json` 中的执行方案。项目文件只配置仓库、任务与 PR 等项目属性；`host`、`agent`、`codex`、`orca`、`model`、`reasoning_effort`、`sandbox`、`profile`、`execution_profiles` 和 `default_execution_profile` 都只能写在全局文件的方案中。每个项目仍有独立的服务和 native Task DAG。

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
      "codex": {"executable": "<本机 Codex 可执行文件>"}
    }
  ]
}
```

按 `s` 打开全局方案列表，`↑/↓` 选择，`Enter` 确认，`Esc` 取消。服务在领取前核对方案 ID、摘要、Task 资格与目标能力；同一 Task 领取时冻结方案。编辑或删除全局方案不会重定向已开始的任务。控制台和服务加载的项目或全局文件内容不同时，控制台阻止新启动和自动派发；修改配置后重启服务并重新打开控制台。

Orca 本机方案可在同一全局文件中配置，五套 Luna 方案见[示例](../examples/execution.json)。使用 `host=orca`、`agent=codex`、`sandbox=workspace-write`，并填写 `orca.executable`、`orca.shell`、`orca.wait_timeout`。要固定模型与推理强度，`model` 和 `reasoning_effort` 必须一起填写，`codex.executable` 必须指向原生可执行文件，`orca.shell` 使用 `powershell.exe` 或 `pwsh.exe`。SteerLine 在 Orca 工作区中用固定命令启动该 Codex agent，启动意图与终端身份分别持久化；不支持在 Orca 方案中再叠加 Codex profile 或其他 sandbox。选择方案后，服务会检查 Orca 可连接且目标仓库已在本机 Orca 注册；检查失败时拒绝派发。受监管远端方案目前仍不可启动。
