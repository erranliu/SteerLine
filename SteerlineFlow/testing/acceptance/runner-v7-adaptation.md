# Runner v7 对接验收（2026-11-01）

本次为用户直接授权的普通开发，工作区基线为 `63c1442e0b239e12576e70cd5d9caececb0fe315` 加本轮修改。不执行安装、旧任务排空或数据清理；未复制至 C:\Project\SteerLine。

## 实现与边界

- 删除公开 verify-takeover 客户端、兼容报告/矩阵、启动/部署/内容生成/环境/恢复中的旧接管依赖；pending_stops 及同构建原资源重启保留。
- 新部署预检使用候选离线 capabilities，要求 state/v7。跨构建升级前查询原服务退出、原资源 verify-stopped 和 Run 业务终态；不把 pr_ready、未知停止或人工待处理作为排空。
- 固定 v7 命名空间按 home 派生，旧 pin 指向其他目录时阻断并保留原现场，要求原版本收尾及新的 InstallHome。无迁移、旧数据读取、自动清空或兼容壳。以后契约兼容且排空的 v7 构建可保留历史，不按每个 Run 的 EXE SHA 判定业务权限。
- 启动前取消通过公开 inspect（冷态 --offline）核对原 namespace/Run/input/request，再持久化原停止信封；只在冷态 request_not_found 时收尾未受理的内容生成取消。未知请求不重发，不重建原资源。
- 原 Run 的 blocked/manual_intervention_required、ResumeRequired、controls 和诊断由现有公开投影/显式 resume 透传，不引入新恢复状态机。
- 删除跨构建活动执行实测脚本及测试生成 A/B 接管变体；保留普通真实测试管理入口，历史验收仅属当时构建。

## 测试指令删除与独立审核

业务原因：新版 Runner 明确退役跨构建活动任务接管，旧实测为制造跨构建服务中断而向测试 Spec/Plan 注入专属指令，已无有效产品入口，整条专属测试删除。没有修改生产固定提示词、组装接线、规则资产或黄金样本。

删除 `scripts/test-runner-update-live.ps1`（Git/LF 原文 UTF-8 字节 20008 → 0；删除前工作副本 CRLF 为 20253 → 0）；实际固定追加正文如下，各项均整段删除，新增正文为空。实际正文 Git 差异随该文件删除可审阅。

追加正文 1：UTF-8 **623 → 0 字节**（包括首尾换行）。

```text

Recovery qualification: in implementation run powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./implement-recovery.ps1 exactly once. It waits 90 seconds to exercise the authorized management-service outage, then writes, verifies and commits result.txt. Use a shell timeout of at least 120000ms, yield at 1000ms and poll that same command to completion. Do not repeat it or implement the file separately. The ignored .recovery-*.json files are test observations; never stage them. Do not modify the script or any other tracked file. After success return the exact completion line: 实施及必要验证已完成。
```

追加正文 2：UTF-8 **183 → 0 字节**（包括首尾换行）。

```text

For this recovery qualification, run implement-recovery.ps1 once instead of doing its operations separately. Allow its 90 second wait to finish; publication remains a separate step.
```

独立只读审核已完成（runner_v7_review）：两段正文及字节数独立核对通过；确认生产固定提示词/材料/黄金样本未改，删除理由成立。首次发现的未完成 operations 漏检、已撤销缺失候选阻断启动、永久关闭失败历史排空误拒三项已修复并独立复核通过，没有新增 finding，代码与测试指令删除获批准。

公开证据边界：`accepted` 表示原操作已有受理事实，联合要求整个 Run 的明确终态、无继续/输入资格、原资源结算，不独立作为操作完成证明；`pending/in_flight/outcome_unknown`、未知状态或不完整/重复操作身份均阻断升级。SteerLine 不解释 Runner 私有 ResourcePending 或按 action 维护兼容矩阵，原版本排空的操作约定仍适用。已永久关闭且资源/操作已结算的 closed/failed 历史不要求变为业务成功。

## 实际验证

所有下列最终报告均为本轮实际执行，工作副本 source_dirty=true；固定 L1/L2 采用最终生产实现，新增 opt-in 真实应用生命周期测试另行独立运行。未导出为干净提交的 Git 测试历史，不将定向集合当作 go.full 或真实模型/PR验收。

原始证据根固定为 `C:\SteerlineDebug\wt-3701-runner-v7`，下表路径相对此根；每个报告保留自己的原命令、选择、原始日志和受测来源。

| 验证 | 实际结果 | 原始报告/证据 |
| --- | --- | --- |
| 固定 L1：`go run ./cmd/test-feedback -level=L1 -out <该次目录>` | passed/complete；准备 0.340s、执行 9.571s | `test-results/L1/20261101T120532-90d5b2b7d1d7479fb3e77d49f6480930/summary.json` |
| 固定 L2：`go run ./cmd/test-feedback -level=L2 -out <该次目录>` | passed，full-L2，cleanup_complete=true；准备 5.816s、执行 109.451s | `test-results/L2/20261101T120543-f33e952291a947b1b1ce5e439f298cd8/summary.json` |
| 部署/停止/中断恢复定向：`./internal/app ./cmd/steerline -run ^TestDeployment -count=1 -timeout=240s` | passed/complete；45/45，零失败/跳过，142.382s wall | `diagnostics/deployment-final-20261101T120053-958a0683949f46939dc95ec375fdde24/summary.json` |
| 启动/生成/命名空间/升级定向：原完整命令及九包选择见该报告 command；保留 240s 原预算 | passed/complete；99/99，零失败/跳过，186.070s wall | `diagnostics/startup-verified-20261101T120500-107e04a5a5a3444a9c310c50f2a7c319/summary.json` |
| 真实 Runner 公开协议：`TestLegacyCheckRetirementPublishedContract` | passed/complete；1/1，零失败/跳过，5.866s wall | `diagnostics/real-runner-final-20261101T120202-d4ad1b57d6e24cd9a967a1fccc14f503/contract/summary.json` |
| 真实 SteerLine applicationRunner：`TestPublishedRunnerV7ApplicationLifecycle` | passed/complete；1/1，零失败/跳过，6.691s wall | `diagnostics/real-application-final-20261101T120910-4f2f72500b3444a98877200e3137ecba/summary.json` |
| Windows PowerShell 5.1 脚本 | `test-deployment-stop.ps1` exit 0；`test-build-deploy-pipeline.ps1` 8 场景通过，仅假安装现场 | `diagnostics/scripts-20261101T115002-c066deaeb5b342458d6a0ca32961eac5/deployment-stop.log`、同目录 `pipeline.log` / `pipeline/` |
| 文档及空白 | 本地 Markdown 导航零错误，`git diff --check` 通过 | 最后本机命令输出；未伪造受管报告 |

真实 Runner 身份：`C:\Project\SteerRunner\bin\steerrunner.exe`，version `0.2.0-dev.4`，source `36de6ae56324b36d89adf240fc1fd26f595f0829`，SHA256 `fa11f9515cb061b6be1b55306f21cc9a9626aff08af81c7360d922d5905c1f2b`，资源版本 3，state/v7。验证前后 EXE 摘要保持；静态升级预检原文位于 `diagnostics/real-runner-final-20261101T120202-d4ad1b57d6e24cd9a967a1fccc14f503/upgrade-preflight.json`。应用生命周期使用全新 home，无业务 Run/模型/PR，核实原服务停止、公开 resources_settled=true、空 Runs 后才移除自有命名空间；失败路径保留现场，独立审核已复核此边界。

### 原失败及补验

1. 首次受限沙箱调用无法正常使用 C:\slr/原生路径，报多项权限失败；其沙箱临时报告目前不可读，不作为有效通过证据。随后仅在已命名的隔离外部 scope 中以正常测试权限执行，未放宽产品校验。
2. `diagnostics/20261101T114411-d3441bda3d314bd6ac8712eea1f3011b/summary.json`：原混合 owner 命令触及 240s 上限，还暴露环境反例仍改 SHA（现应改 namespace）和保存前后比较未使用规范化存储快照；两项测试已校正。
3. `diagnostics/targeted-final-20261101T115511-2e5fcae895b742ce85c4f8bc7e986c4f/summary.json`：无此前断言失败，但混合部署、启动及生成选择仍超 240s；保持预算，按部署与启动/生成拆成上述两次独立完整执行，不拼接超时轮次为通过。
4. `diagnostics/startup-final-20261101T120052-9def51c16f514aeaaa842389ef1bfb10/summary.json`：98/99 通过；驻留 capabilities 故障测试被误改为要求静态预检先失败。静态命令不读运行目录，已恢复原实例绑定与失败后结算断言，随后整个启动组重新完整通过（99/99），不是只取单项补测拼成通过。
5. 新增真实应用测试首轮成功后，独立审核指出 mock fixture 有无条件 RemoveStopped 注册，会破坏失败时保留现场；改为独立 home/最小配置、任一停止失败即返回、成功公开证明后才清理。修订后的真实应用测试独立完整复跑通过，最终审核批准。早期成功报告保留于 `diagnostics/real-application-20261101T120703-5704ad8ec5c841da866cc09205054871/summary.json`，最终以表中修订后报告为准。

未执行安装维护、排空或清理任何旧业务、真实模型/PR链路或完整 go.full。旧安装须继续用原版本收尾，升级操作见[安装说明](../../../docs/integration/install-deploy.md#同构建更新与重启)。
