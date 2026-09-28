# 预设执行配置

一个仓库仍使用一个 SteerLine 服务、一套 native 任务 DAG 和按执行身份管理的资源占用。项目 JSON 顶层的 `host`、`agent`、`codex`、`orca`、`model`、`reasoning_effort`、`sandbox`、`profile` 与 `project_steps` 构成内置的 `project-default` 方案。可在同一项目 JSON 的 `execution_profiles` 数组中预先增加命名方案；控制台只允许选择这些方案，不接受临时参数。

每个附加方案须完整指定 `id`、`name`、`host`、`agent`、`sandbox`、`codex`、`orca`（Orca 方案）和该方案的 `project_steps`。不同任务可选不同方案；同一任务从 implement 到 publish_pr 固定使用领取时所选方案。`project_steps` 中每一步的 `execution_site` 必须与方案后端一致：本地 Codex 为 `local`，本机/受监管远程 Orca 为 `target`。步骤 provider、入口和检查清单仍须满足 minimal-v1 规则。

示例结构（将 `project_steps` 展开为项目实际的四个固定步骤）：

```json
{
  "host": "local",
  "agent": "codex",
  "model": "",
  "sandbox": "workspace-write",
  "project_steps": {"implement": {}, "verify": {}, "review": {}, "publish_pr": {}},
  "default_execution_profile": "project-default",
  "execution_profiles": [
    {
      "id": "local-strong", "name": "本地 Codex 高强度",
      "host": "local", "agent": "codex", "model": "<明确模型>",
      "reasoning_effort": "xhigh", "sandbox": "workspace-write",
      "codex": {"executable": "<本机 Codex 可执行文件>"},
      "project_steps": {"implement": {}, "verify": {}, "review": {}, "publish_pr": {}}
    },
    {
      "id": "orca-local", "name": "Orca 本机",
      "host": "orca", "agent": "codex", "model": "<明确模型>",
      "reasoning_effort": "high", "sandbox": "workspace-write",
      "codex": {"executable": "<目标可执行文件>"},
      "orca": {"executable": "orca", "shell": "cmd.exe", "wait_timeout": "15m"},
      "project_steps": {"implement": {}, "verify": {}, "review": {}, "publish_pr": {}}
    }
  ]
}
```

此片段说明字段布局，不是可直接运行的完整配置。受监管远程方案使用 `host=orca_supervised`，并按现有远程配置契约填写 `orca.supervised` 的固定连接、环境、仓库 selector、协调终端、轮询与能力报告版本；model 和 reasoning_effort 必须明确。远程与本机 Orca 在控制台使用相同的选择和任务阶段流程，取消能力等平台差异由后端如实报告，绝不隐式回退。

当前 native `agent_skill` 步骤只支持 Codex。可以预设其他 Orca agent 供纯命令步骤使用；若固定步骤仍含 `agent_skill`，控制台会标为不可用。平台能力、CLI 版本和远程连接在领取前由服务再次核验；配置存在不等于目标已认证。

按 `s` 打开方案列表，`↑/↓` 选择，`Enter` 确认并提交，`Esc` 取消。服务核对方案 ID、内容摘要、服务实例、控制版本、任务资格及目标能力后才领取。方案选择和整个执行路线固定在 WorkflowRun；编辑或删除配置不重定向已开始的任务。修改项目配置后重启服务和控制台。自动派发不能等待人工选择：存在附加方案时，必须显式设置 `default_execution_profile` 才能切换自动模式；留空表示只允许手动派发。

台式机64g 的有人值守交付配置及操作边界见[远程执行 MVP](remote-execution.md)。远程方案为附加命名方案，不隐式改变项目默认后端。
