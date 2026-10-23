# 历史远程 Orca worker 配置与协议

> 保留配置/协议资料，非当前第一阶段操作入口。第一阶段只含本地 Codex 与本机 Orca；远端新 worker 准入尚缺目标权限证据，本节不表示该流程可交付或已通过验收，也不构成本机任务的前置。

远程方案与本地 Codex、本地 Orca 共用 Task、Decision、Operation 和 PR 交付规则。Run 与协调 inbox 位于中心，工作区与 agent 位于指定远端。首次创建远端工作区和 worker，后续轮次复用同一终端与 incarnation；不创建本地替代执行。

在全局文件的 `execution_profiles` 中添加以下方案，替换连接、仓库与协调终端的真实标识：

```json
{
  "id": "orca-remote",
  "name": "远程 Orca Codex",
  "host": "orca_supervised",
  "agent": "codex",
  "implementation_configs": [
    {
      "implementation_key": "codex.worker-profile",
      "codec_version": 1,
      "payload": {"executable": "<远端使用的 Codex 命令>", "model": "<明确模型>", "reasoning_effort": "high"}
    },
    {
      "implementation_key": "orca.worker-profile",
      "codec_version": 1,
      "payload": {"executable": "<本机 Orca 原生可执行文件>", "shell": "powershell.exe", "wait_timeout": "2m"}
    },
    {
      "implementation_key": "orca.supervised-profile",
      "codec_version": 1,
      "payload": {
        "connection_id": "<environment list 返回的 pairedDeviceId>",
        "environment_id": "<已保存的 environment ID>",
        "repository_selector": "id:<远端 repo ID>",
        "coordinator_terminal": "<中心已打开的 bootstrap terminal handle>",
        "poll_interval": "2s",
        "capability_report_version": 1,
        "remote_cancel_available": false
      }
    }
  ]
}
```

远程协议与身份链路保留，但目前 worker-start 未返回绑定实际目标/版本的文件、网络及越权策略证据，所以所请求 execution_policy 在领取前明确拒绝。原先的连接/版本族/remote/federation 检查仍属于远程适配；不能据此推定权限落实，不降级到本机。配置中的协调终端用于预检，每次 WorkflowRun 另建独立的中心协调终端。远端必须已具备 Codex、项目工具和 PR 发布权限。

原生 question 进入共享 Decision；活动 Dispatch 的反馈通过消息投递，已结算轮次的反馈通过原终端开启下一轮。输入回执丢失时只核对原 Task/Dispatch 或 question history，不重复启动或回答。中心重启保留原执行并重新核验，旧服务保存的未投递输入需要重新确认；断连或身份变化保留资源并显示未核实。远端路径按执行主机返回值保存，不按中心操作系统转换。

`worker_done` 只证明轮次结算。提供方证明对应 PR 已合并后才写 TaskDelivery、关闭 Task，并 release 已结算 worker 和关闭排空的中心协调终端；远端 worktree 保留。远程取消仍不可用，停止中心监督不会被显示为远端进程已经退出。

原记录日期：2026-09-30。隔离协议与 Service 测试不证明目标权限落实；当时保存的“台式机64g”连接不可达，未完成双机真机验收。当前支持范围以产品设计和实际能力核验为准。
