# 调用方边界收口验收

2026-10-20，基线 `0752537e`，用户直接授权；[实施 Plan](../plans/architecture-boundary-consolidation.plan.md)。

| 自动项 | 验收范围 | 结果 |
| --- | --- | --- |
| ABC-A01 | HTTP 基础设施窄依赖及新调用方模块直接/间接/全标签守卫 | 通过：boundary 173 项，86.206s；Windows/Linux 生产图、未激活标签、Task/宿主原守卫及新调用方 owner 反例均通过 |
| ABC-A02 | 唯一 stop 信封准备、先持久化、CAS/失败、未知和明确拒绝、原对账语义 | 通过：core 87 项通过/1 项显式跳过，14.055s；保留正常随机 ID 与启动确定 ID，未知/重复准备原字节不变，保存失败/过期快照不产生可派发 stop，正常对账消费原启动信封，公开 JSON 保持 |
| ABC-A03 | 统一 takeover 公开校验/原事实匹配、启动/离线/查询/部署消费及资源 owner 回归 | 通过：app 定向 26 项，320.683s；CLI/固定选择登记 36 项，50.045s；身份规则最终补测 9 项，77.019s；部署脚本、PowerShell Parser 和差异检查通过 |

## 实现与保持的边界

- stop 唯一构造/持久入口为 `runnerbridge.prepareStop`，`PrepareStartupStop` 在候选 gate 内重读原 revision，并在返回 pending_stops 前保存；app 继续持有项目锁/store 并控制启动顺序。TaskDelivery/事务和 stop 受理后的原生结算均未移动。
- takeover 报告、启动输入及原请求 digest 归 runnercontract，无旧类型别名；runnerbinding 只依赖 runnercontract/runnerstate。启动准入与离线重启分别保留原有未知提交前置，匹配不会改写 Run/input/PR/原 submit、历史 build 或检查 selection。只读报告不获得新输入/停止权限。
- 调用方 `Compatible` 保持 SHA 精确比较与按宿主 OS 的路径比较；wire 校验的格式接纳不放宽调用方身份。最终补测专门覆盖这个差异，并复测实际兼容启动与启动 stop outbox。
- 部署在线预检通过暂存 SteerLine 的 `verify-runner-takeover --runner-exe <candidate> --runner-sha256 <sha> --state-dir <original> --json` 复用公开 Go 校验；仍在停旧服务/替换文件前执行。原 `verify-restart` 与停服/资源结算分别核验的门槛保持。

## 实际验证与证据

Windows/amd64、Go 1.26.4，当前工作树增量；所有现场由既有隔离测试夹具创建，无真实模型或安装环境操作。命令为 Plan 的 boundary/core/app/cli-selection/identity-final 五组及 `scripts/test-deployment-stop.ps1`。证据根目录：

`C:/Users/lmas1/AppData/Local/Temp/steerline-boundary-consolidation-03c1c355c95c48dea942170234e37b43/`

各组的 `summary.json`/`go-test.jsonl` 保存实际选择、运行状态和原日志，脚本日志为 `deployment-script.log`。core 跳过的唯一项为 `TestEnvironmentCheckPublishedContract`：未提供 `STEERRUNNER_EXE` 的真实发布契约交接，按既有规则显式跳过，不计入本轮自动项成功或真实联调证据。

审查基线中 `projectconnection -> processwindow` 导致的守卫失败已修复；没有放开业务/SQL/具体存储依赖。实现期间首次只编译发现迁移后 runner_build_binding 的未使用 import，删除后后续验证通过；其余上述实际运行均通过。测试临时资源按各夹具原有 cleanup 回收，报告保留。

仅定向隔离自动验证，无人工项；不宣称完整 L1、L2/L3/L4、Full、真实原生接管或部署通过。提交、推送及 PR 按用户后续授权执行，合并和部署不在本次范围。

## 提交前自查（2026-10-20）

逐项对照 `0752537e` 的原生产实现，复核两种 stop ID 生成、先保存后派发、未知原请求/正文/revision、明确拒绝后的准备、迟到 Run 关联、原构建/目录/Check/input digest、只读接管、原服务退出与资源结算分别证明、部署预检在停服/替换之前以及应用/项目资源 owner。Task/DAG/Delivery、数据库 schema、原 CLI 信封、授权、重试及原生流程均未变更。

本轮自查修正三项：

1. `SamePath` 恢复调用方原有的 `filepath.Clean` 后 Windows 转小写比较。`EqualFold` 对部分 Unicode 字符会扩大原目录等价范围；新增 final sigma/sigma 用例防止改变原 Runner build/state-dir 身份。
2. 补验 `app-identity` 首次运行 4 项通过/1 项失败，65.887s：`TestStartupCancellationOutboxIsDurableBeforeRunnerStart` 在 `seedStoppedPublicRun` 准备阶段，由测试临时 bridge 与真实 actor 同时写候选而触发 `stale_version`。改为只由原 actor 观察公开 fixture，测试有界等候其已提交 Run/input/cursor/stopped 事实；保留生产 CAS，零命令重试。随后 `app-fixture-final` 中一个重启消费者在 fixture 控制回执准备时超过 8 秒；改用冻结宿主的既有 `WaitControlReceipt` 核对原信封/回执，避免逐次重建配置/Git 身份，仍保持原期限。补测覆盖该公用夹具全部调用者。
3. `app-fixture-final` 还暴露旧 `reject_changed_build_in_old_environment` 用例仍强制要求兼容接管能力合入前的 `runner_build_changed` 文案。实际项目已为 failed、Runner 保持退出，该断言却误报放行。用例更新为公开 fixture 明确返回不兼容报告，并核验拒绝、未启动替代服务及原 home build pin 原字节保持，不修改生产接管门槛。

复核证据根目录为 `C:/Users/lmas1/AppData/Local/Temp/steerline-boundary-pr-review-45c47ff1a4e3459da00883557eb5b2f9/`：core 88 项通过/1 项原真实契约检查跳过，11.031s；boundary 定向 69 项通过，14.372s；部署脚本及 Parser/格式/差异检查通过。`app-identity` 和 `app-fixture-final`（301.586s）保留上述失败，最终修正补测分别保存到 `app-fixture-verified` 与 `incompatible-takeover-final`，不覆盖失败日志。

最终复核通过：`app-fixture-verified`（222.233s）验证修正后的全部公用夹具消费者、显式重启正反例、启动撤销、原绑定与部署身份；`incompatible-takeover-final`（34.267s）验证明确不兼容时拒绝且原 pin、原退出服务保持。ABC-A01–A03 无剩余自动验收失败；未发现其他业务或恢复语义差异。本结论限上述源码核对和隔离定向验证，不扩展为真实原生/部署资格。

已快进同步 `origin/main` 的 `551064f3`（PR #361），仅四份架构/执行链文档，未引入生产代码或脚本变化，也未改写其规则。
