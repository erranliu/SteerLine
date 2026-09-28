# H-A16：SteerlineTest 原生轮次的可见 TTY 折叠

2026-09-28，Windows/amd64；SteerLine 安装二进制 SHA-256 `be7b242af31541d2460fb46c37de9bdb4077e9e4618b642852b0d7dcb59567df`，本机 Codex CLI `0.158.0-alpha.2.1`。使用原已启动的 `SteerlineTest` 服务和此前未领取的原生 Task 104。所选为项目默认本机 Codex high；新建的观察终端只运行 `steerline console -config C:\SteerlineRuntime\etc\SteerlineTest.json`。没有重放已关闭的 Task 101–103。

- 原 Task 104 / WorkflowRun `0b87006230fd0c6106b1d0d215fb7fa0` / Execution `b6203dcfe4266e44a01b76ef0661974c` / implement Attempt `8bfdfe780a4f4435d7ca2ce4d16c2cf2`。中心 SQLite 的 `codex-app-server` 原生 turn `01a0e6d2-18f7-7a02-86c2-e3d51b792212` 同时有 `turn.started`、`turn.completed`，以及同一 turn 的 `message.completed` / `phase=final_answer`。最终正文完整，1349 字节，SHA-256 `a3ff6429d6efd247d537a6c78e6bcbf16bf0cbae9b1612ec5c232fc74da701f7`。
- [自动折叠画面](auto-fold-screen.json)显示已结束轮次 `▸`、37 个过程项收起，`final_answer` 和 `completed` 终态仍在折叠外；未知轮次的提示单独保留。[手动展开画面](manual-expand-screen.json)显示同一轮次 `▾` 与 commentary/工具项；再次按 `e` 后的[折叠画面](manual-recollapse-screen.json)保留 final。按 `n` 的[最终正文画面](final-body-screen.json)定位到原 Execution/Attempt 的完整正文引用，未把步骤结果或最后一行终端文本推断为 final。
- Task 104 的五步验证完成后，[PR #22](https://github.com/erranliu/SteerlineTest/pull/22) 只新增 `evidence/native-104.md`。提供方固定 head `ac2a22c7c1213defd10cc534ea5df7a37e4aee21` 合并到 `main`，merge commit `2dc678c7291119515d0c294b4762aac63b95cf1b`；原中心 Delivery 使用同一 PR 与 merge commit，Task 为 `closed`。观察控制台按 `q` 退出后，SteerlineTest 服务仍 `running/ready`。

这完成 H-A16 的**真实本机 Codex 精确轮次折叠与导航**门槛。Task 104 没有未决业务 Decision；折叠不改变 Decision 的状态由既有状态机和 UI 自动化覆盖，此次不冒充一条新的真机未决问题样本。#3 的 Orca 平台自身同句柄新 incarnation 尚未观察，仍单独开放。
