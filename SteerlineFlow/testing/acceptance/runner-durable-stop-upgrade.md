# Orca 已确认停止后的 Runner 升级（2026-11-03）

用户要求在 worktree 修复 SteerRunner 的停止证据与升级核验，并在验证后部署 `C:\SteerlineInstall`。原部署在替换文件前失败：远端环境检测 Run `fd4cdfaec82b6b5a5017adce97590f18` 已 closed/stopped，但原 Orca runtime 已变化、原 worktree 不在清单中，原终端查询返回 terminal_handle_stale。仅 stopped 标签不足以证明终端停止；此次使用原事务已保存的精确 confirmed 停止回执，不清库、不修改历史 Run，也不根据工作区消失推断停止。

## 实现范围

- Runner worktree：`C:\Users\lmas1\.codex\worktrees\runner-durable-stop\SteerRunner`，分支 `codex/durable-stop-proof`，基于 `457ffaea20b6227a0bdaec9e56f5b9e7f54da337`；修复提交 `40f83cb984193c72ca15944f10fccc11f5327d24`。
- SteerLine worktree：本目录所属 checkout，分支 `codex/runner-stop-upgrade`，基于 `195a411a0875300bb9514ff584307ce5f05d6f9d`；实现提交 `5ecc0ff0`。
- Runner 验证原 stop operation、原 Run/input、attempt/generation、session、runtime/worktree/terminal/incarnation 及 confirmed/Stopped/WorkerAbsent/confirmed progress。已确认停止可离线验证并在对账时重放；缺失、accepted/unknown、冻结范围或身份不符的回执不能获得该证明。
- SteerLine 用已校验候选的公开 verify-stopped 核对同一 v8 目录，再由原 EXE 和候选 EXE 分别 inspect 同一 Run/input 集合；原服务退出、业务终态和未完成控制门禁继续生效。接线覆盖升级预检、离线重启核验和中断部署恢复。
- 不新增 schema、迁移或旧格式读取路径。证明范围保持 orca_terminal，不扩大为后台后代进程或凭据已清理。未修改固定提示词、正文、材料、黄金样本或提示词守卫。

## 验证证据

原始证据根：`C:\SteerlineDebug\runner-durable-stop-20261103`。每次执行使用独立目录；初次失败保留，不覆盖。

| 范围 | 结果与入口 | 原始证据 |
| --- | --- | --- |
| Runner 定向停止回归 | application/orcatransport 的正式停止、回执重放及原未确认停止回归通过 | 工具执行记录；完整 L1 同时包含这些用例 |
| Runner 完整 L1 | passed；781 个顶层、29 个扩展测试通过；原生 driver 编译及零匹配守卫通过，l1-core prerequisite=passed，1069.481 秒 | `runner-L1/summary.json`、`prerequisites.json`、`l1.jsonl` |
| Runner 公开服务归属与停止核验 | `scripts/test-foundation.ps1 -Pattern '^TestSRServiceOwnership$'`，1 个顶层、全部子例通过；覆盖远端重启、worktree 移除、离线、缺回执、accepted/unknown、身份/范围冲突、零原生变更及 SQLite 字节不变 | `controlled-final/result.json`、`integration.log`、`source-identity.json` |
| SteerLine 固定 L1 | passed/complete | `steerline-L1-final/summary.json`、`managed-l1.minimum.json` |
| SteerLine 固定 L2 | passed、cleanup_complete=true | `steerline-L2-final/summary.json`、`managed-smoke.delivery.json` |
| SteerLine 客户端全集 | passed/complete | `steerline-client-final/summary.json` |
| SteerLine 应用相关回归 | 初次定向报告中的 app 包全部通过，169.354 秒；含升级、离线核验、中断部署恢复。该次整体因新客户端夹具缺 revision 失败，不能记作整体通过 | `steerline-targeted/summary.json`、`go-test.jsonl` |
| 新升级客户端矩阵 | 修正夹具 revision 后 11 个子例全部通过；随后客户端全集通过 | `steerline-upgrade-final/summary.json` |
| 真实安装只读升级预检 | 原版本业务核验与候选资源核验通过，原 15 条 Run 全部 resources_settled=true | `installed-preflight.json`、`installed-native-proof.json`；候选来自 controlled-final |

初次公开停止回归失败于新增反例的查询/范围期望：实际实现已在身份不符时提前拒收，测试还要求发生 inventory 查询；修正夹具期望后重跑通过。初次 SteerLine L1 检出 runnerclient 新增对 runnerbinding 的越层依赖；实现改用标准库核对绝对路径，保留原分层守卫并完整重跑 L1/L2 通过。

完整 L1 在基线提交加本次未提交修复上执行，源码与最终修复一致；报告保留原 source_commit/source_dirty，不改写受测身份。三份原安装库均为 schema39，与当前 SteerLine 一致。不宣称完整真实模型交付或远端全场景资格。

## 实际部署与启动

2026-11-03 15:20（Asia/Shanghai）通过正式 `scripts/build-deploy.ps1 -UpdateRunner -RunnerRepo <上述 Runner worktree> -InstallHome C:\SteerlineInstall -NoPause` 完成安装并启动；报告出口为证据根 `deploy-timings.json`，原日志 `deploy.log`。总耗时 61.080 秒，升级预检 5.740 秒、全部项目 ready 等待 23.320 秒，succeeded=true。

安装包为 `C:\SteerlineInstall\deployments\20261103-072005-11aafc64`。SteerLine 来源 `e213a6fcb673e77977f2b19a6c698f372123f418`（实现加本验收记录的部署前版本），source_dirty=false；Runner 来源 `40f83cb984193c72ca15944f10fccc11f5327d24`，version `0.2.0-dev.4`，SHA256 `3daffb75460cb0ed829d5396c366f11148c69fd39626aa5a0f6c3daf8c03db8a`。三个安装 EXE 均与部署记录摘要一致，原 v8 namespace `C:\slr\rehnosf7lwgci2qaqusuz6z56u` 保留，未清理或修改历史 Run。

15:21 的部署后核验确认：后端 PID 39868、Web PID 51148、Runner PID 43804，均 ready；SteerLine、SteerlineTest、merge-h-308-code 三项目 ready/online，degraded=false。`http://127.0.0.1:18742/` 返回 HTTP 200。全部项目配置及 Web 配置原始字节摘要不变；执行方案摘要不变，8 个 profiles、默认 project-default 和 developer 配置均保留，仅按正式部署流程更新 Runner pin。

证据根中的 `installed-before-hashes.json`、`installed-profile-digest.json` 记录部署前摘要；`installed-status.json`、`installed-runner-status.json`、`installed-after-verification.json` 保存启动、Runner 身份与配置保留的最终核验。部署包另有 `shutdown-verification.json`、`runner-upgrade-preflight.json`、`deployment.json` 和本次 startup 日志/回执。源码原 checkout 未应用修复，两个修复分支和 worktree 保留。
