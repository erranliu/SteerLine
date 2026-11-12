# 308 环境检测提交回执恢复验收

2026-11-12。SteerLine 基线包含 #456，Runner 基线包含配套 #117。308 原环境检测记录 `4ee2990b037c4f409ba1fa68725e54b8` 在 Runner 提交 `77b23682805a4d13f0aadb60d4a7b5d5` 的回执发送失败后持续显示未知。Runner 日志核实收到了请求，随后查询没有已保存命令。原始回执内容未留存。

配套 [SteerRunner PR #154](https://github.com/erranliu/SteerRunner/pull/154) 持久保存受理前的原请求。SteerLine 继续查询同一 ID，显示准备中状态，绑定已接受的 Run，或用持久拒绝及其原错误结束本轮。冷态启动可结算无 Run 的准备中命令。首次提交错误仍保留。受理前新增的诊断没有改动模型输入。

## 回归

证据根为 `C:/SteerlineDebug/wt-2db3-submit-receipt`。

- Rebase 后的 `8e8b8963` 执行固定 L1、L2，均通过。L2 清理完成。报告分别在 `test-results/L1/rebased-8e8b8963/summary.json` 和 `test-results/L2/rebased-8e8b8963/summary.json`。
- 正式环境专项 `go.environment` 在接入 #517 之前通过，见 `test-results/L4/76a6a774/summary.json`。最终后端的环境、提交、接管与恢复定向运行通过，见 `diagnostics/steerline-final-owners/summary.json`。
- Runner 的 storage、host 和 application 定向回归通过。真实 CLI 存储故障用例 `TestSREdgeStorageFailure` 通过，故障恢复后同 ID 只执行一次。报告在 `diagnostics/runner-receipt` 和 `diagnostics/runner-cli-storage-failure`。

## 308 本机

独立 clone/home 使用 `merge-h-308-code` 的 `AIRefactor@d2eee8a6487ea076d8bba65fa52589a683611143`、本机 Codex `project-default`、正式 `environment_start`。记录 `df29a8b7450744a8bd1000376aa5ea33`，Run `e884df58e817ed6ab4806c78e7dcf230`。

北京时间 13:06:33 达到 `ready`。十项均通过，包括 Android Editor 模块和 Unity 批处理授权。Run `completed/stopped`，最终报告 `closed`，独立服务停止且资源已核实结算。报告、状态观察、构建摘要和停服证据位于 `live-308/attempt-1/evidence`。原回执断连由回归用例覆盖。此次真机验收使用 rebase 前构建，已核对 rebase 后的后端源码树字节一致。
