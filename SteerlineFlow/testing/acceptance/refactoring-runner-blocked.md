# 重构任务的 Runner 阻断展示与 Codex 启动诊断

2026-11-19，源码基线 `3a1777a6bbf5d4b245be2ca12aa24c4b2af4e905`，在当前 worktree 修改并验证。安装版及原任务保持只读取证，未部署、停止、恢复或重新提交原任务。

## 原现场与根因

- 项目：`merge-h-308-code`，重构任务 `f292bdcfcf4b4854ad10819f0c4834a9`，操作 `913859abc07b9882bae51bbe6f03120f`。
- 原 Runner Run：`d9f8ce00c99db0f6f5a8ede27dea75e3`，安装 Runner 源码 `8ebbc717776eb2ad85371060566776c4402773fa`，Codex `0.166.0`。
- 北京时间 11:29:06.732 创建 Codex 进程；11:29:18 的 `prepare_session` 记录 app-server 固定端口 `53895` 连接被拒绝；11:31:25 Runner 对账耗尽，进入 `blocked / manual_intervention_required`，`turn_started=false`。
- 原生 SQLite 的 `backfill_state` 在 11:37:36 才完成；该次新库的 `threads` 有 8,033 行，全部指向用户 `.codex` 中的历史 rollout。原生日志在 11:37:37 记录端口开始监听，距进程创建约 8 分 30 秒。原进程后来就绪，不代表原任务已恢复。

安装 Runner 的 `internal/execution/adapters/codex/launch.go` 给每个原生服务指定全新的 `app-server/sqlite`，但保留用户 `CODEX_HOME`。Codex 同版本的[启动入口](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/app-server/src/lib.rs#L634)在开启 WebSocket 监听前初始化状态库；[状态库初始化](https://github.com/openai/codex/blob/rust-v0.166.0/codex-rs/rollout/src/state_db.rs#L87)等待历史会话回填。现场完成时刻、数量与这条调用链吻合，解释了监听前的长耗时。模型、选题方向及目录信任警告均不能解释这个等待阶段。

只读汇总：`C:/SteerlineDebug/wt-9ed5-refactoring-blocked/diagnostics/native-startup-evidence.json`。只保存时间、计数与来源，不复制历史正文或认证信息。独立空 home/SQLite/目录/端口的两次无模型探针在约 2.1 秒、1.0 秒就绪，第二次带用户配置快照；其自有进程均已停止，证据为 `diagnostics/startup-09c1b89200c9427bae1f7824ff4dd714/summary.json`。

完整复用原进程环境的追加探针被自动审批拒绝，未执行。随后通过原生 SQLite 的只读统计与同版本源码完成定位，无需复用认证或代理凭据。未在本次修改 Runner 的 SQLite/home 隔离策略，也未延长超时掩盖历史回填。

## 修复行为

原操作未结算且最新已绑定 Run 为 `blocked` 时，摘要和详情显示 `blocked`，诊断包含 Runner 的总体及原操作错误并去重；操作详情另提供 `run_diagnostic`。不把历史 Run 的阻断或已经停止的 Run 当成当前在途阻断。

这是现有已验证 Run 的展示投影：保留原存储阶段、Run ID、操作的 `submitted` 状态、在途名额、持续观察及停止责任。禁止在该状态提前交付；仍可明确停止。停止意图优先显示“停止待结算”；后来原 Run 的状态变化可正常清除阻断投影，不自动发送 resume、替代提交或新模型输入。

页面的执行对话显示“已阻断（待结算）”及实际诊断，复制调试信息包含 Runner 状态、诊断和原 Run ID。无提示词、材料、黄金样本或数据库结构变更。

## 验证

- Go：`diagnostics/go-blocked-01/summary.json`，query、refactoring、refactoring/model 三包全部 owner 测试 212 项通过、零跳过。新增回归覆盖各阶段摘要/详情、空诊断、历史/停止优先级、保持未结算责任、拒绝提前交付、持续观察和明确停止；无自动新输入。
- 前端：重构页面组件 33 项通过，原生 JSON 为 `diagnostics/refactoring-web-01.json`，含 generate/execute 两种在途阻断、可见诊断、调试复制和按钮资格；`npm --prefix web run build` 类型检查及生产构建通过。格式化后撤销了无关格式改动，不改变已验证语义。
- L1：`test-results/L1/20261119-blocked-01/summary.json` 通过，准备约 0.6 秒、执行约 83.3 秒；实际总耗时超过一分钟目标，如实保留原报告。
- L2：`test-results/L2/20261119-blocked-01/summary.json` 通过，准备约 39.9 秒、执行约 249.0 秒；独立 smoke.delivery 场景的交付与清理完成。
- go.http：`test-results/L4/20261119-go-http-01/summary.json` 通过；在 L2 完成后串行运行正式七包 HTTP/快照 owner 集合。
- 固定集合与专项：执行结果登记在下方。所有输出均位于 `C:/SteerlineDebug/wt-9ed5-refactoring-blocked/`，不使用安装环境作调试现场。

文档链接检查：819 份 Markdown、5,009 个本地链接，零错误；`git diff --check` 通过。所选验证均已完成。组件测试不代表安装页面已更新；原生启动隔离策略的修复与原任务恢复不在本次执行范围。

## 后续真实完整重构测试

2026-11-19 按用户追加要求，使用当前 SteerLine 源码及 Runner 启动策略修复后的固定 EXE 执行正式 `refactoring.local-codex`。不部署，不操作原安装任务。Runner SHA256 为 `71fbe6336366e87181b5a735dd2ea201ecc3542189889339995b7dcdacd1fa6e`，生产源码摘要为 `41911f53520786a67d85da4ced92a7941a17796d8334a9f44e0970f7b5bde20e`。

```powershell
$env:STEERRUNNER_EXE = 'C:/SteerlineDebug/wt-9ed5-runner-startup/artifacts/steerrunner-index-reuse.exe'
go run ./cmd/test-feedback -level=L4 -suite=refactoring -env=live -scenario refactoring.local-codex -out C:/SteerlineDebug/wt-9ed5-refactoring-live/test-results/L4/local-codex-01
```

- 正式 `summary.json`：passed=true、cleanup_complete=true，1 项通过、0 失败、0 跳过；准备 20.55 秒、执行及报告收尾 560.67 秒，总计 581.51 秒。场景正文及清理 544.77 秒。
- 真实 Codex 环境检测、初始选题、实施、验收、draft PR 发布和独立评审均完成。同项目两条维护任务以不同 Session 重叠运行；各自绑定独立 worktree、分支和 PR，并在接受后才创建 PR。
- 本轮专用 [PR #851](https://github.com/erranliu/SteerlineTest/pull/851) 与 [PR #852](https://github.com/erranliu/SteerlineTest/pull/852) 均以独立测试分支为 base。推进该 base 后，alpha.cjs、beta.cjs 均实际产生 Git 内容冲突；续作在原 worktree 更新 #851，绑定新 head/base，生成第二份独立评审并保持 PR 正文。两版代码的 `node check.cjs` 均实际通过，保留并发修改注释。
- 第二份评审建议为 `insufficient_evidence`：评审材料缺少测试执行证据，评审未据此建议合入。正式场景验证独立评审与 PR 版本绑定，不要求模型必须推荐合入；这次通过也不宣称评审推荐合入。
- `scenario.json` 清理无错误，Runner 清理 confirmed=true；本轮 PR 均已关闭、自有分支已删除，隔离 home/服务已清理。另只读核对两条 PR 均 CLOSED、专属 base 前缀 refs 数为 0。

全部原始证据位于 `C:/SteerlineDebug/wt-9ed5-refactoring-live/test-results/L4/local-codex-01/`，重点为 `summary.json`、`scenarios/refactoring.local-codex/refactoring-live-proof.json`、`maintenance-native-overlap.json`、`conflict-proof.txt`、两版 report/tree-check 和 `scenario.json`。本次不更改测试定义或提示词，也不替代 Runner 原失败批次的完整 L1/Controlled 资格。
