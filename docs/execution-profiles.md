# 预设执行配置

一个仓库使用一个 SteerLine 服务、一套 native Task DAG。生产配置固定 `worker-flow-v1`；顶层 `host`、`agent`、`codex`、`orca`、`model`、`reasoning_effort`、`sandbox`、`profile` 构成内置的 `project-default` 方案。项目 JSON 可通过 `execution_profiles` 预设命名方案，控制台不接受临时执行参数。

每个附加方案指定 `id`、`name`、`host`、`agent=codex`、`sandbox`，以及后端所需的 Codex/Orca 设置。项目步骤、中心 verify/review、修复轮数和 dirty 忽略规则不属于当前配置。同一 Task 领取时冻结所选方案；后续反馈仍使用该 Execution、工作区和 Codex session。

```json
{
  "task_authority": "native",
  "workflow_protocol": "worker-flow-v1",
  "host": "local",
  "agent": "codex",
  "sandbox": "workspace-write",
  "default_execution_profile": "project-default",
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

此片段展示方案字段，完整项目配置还需仓库、remote、base 分支、PR provider 等字段；见 [示例](../examples/config.json)。本次只开放 local Codex 方案。Orca 本机与受监管远端适配代码保留，但配置中的这两种方案会被标为不可启动，服务在首次派发前拒绝；其存在不代表已交付能力。

按 `s` 打开方案列表，`↑/↓` 选择，`Enter` 确认并提交，`Esc` 取消。服务在领取前核对方案 ID、摘要、Task 资格与目标能力。编辑或删除配置不重定向已开始的任务。存在附加方案时，自动派发需要显式设置 `default_execution_profile`；留空时只允许人工选择。
