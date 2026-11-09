# 按 workflow 配置模型估价

日期：2026-11-09。用户授权普通开发，当前工作区实施；不纳入 roadmap，不部署、重启或改写安装现场。依据[当前 Spec](../../spec/billing-ownership.spec.md)和[Plan](../../plans/billing-ownership.plan.md)。

## 契约与实现

只读核对 `C:\Project\SteerRunner` 合入提交 `9243a79b`（#133），其 `docs/model-usage.md` 和 `internal/usage/api/api.go` 发布 frozen_stage_input 能力及 step_id/configured_model/configured_reasoning_effort。原 `bin/steerrunner.exe` 尚为旧构建，不能用它证明新契约。

SteerLine 按原阶段配置模型计价，不依赖 observed provider/model、metadata_basis、call_time 或观测元数据冲突。保留完整分页、计数及 Token 一致性校验。匹配精确配置模型及候选持久化 CreatedAt 所在价表时段，多个计价供应商同时适用时报告 PRICE_AMBIGUOUS；缺配置归属不从当前 profile、首个模型或观测模型补值。输出按阶段/模型/推理强度分组的金额，标注参考估算和主执行会话范围。删除未接正式契约的 interval 计算分支；价表格式、采集开关、执行和交付规则不变。无提示词正文、提示词接线或黄金样本变更。

## 验证计划与结果

所有原始证据放在 `C:\SteerlineDebug\wt-8a73-configured-billing`，每次调用使用新目录。工作区从 `221812ec` 快进到已合入主线 `5f2502ee9979dc269539f253334ae6b76948e93d`，纳入 #474 默认远端采集前置；最终受测源码为该 HEAD 加本次差异。

初始基线的 `diagnostics/owner-initial` 定向36项通过，`test-results/L1/20261109-final` 和 `test-results/L2/20261109-final` 通过，均不冒充同步主线后的最终结果。`diagnostics/published-usage` 首次联验失败：旧 usage_policy.remote_log_access 被新 Runner 严格契约拒绝，服务未启动；同步 #474 既有修复后复验，保留原失败。

独立 Runner 构建：`runner/steerrunner.exe`，version `0.2.0-dev.configured-billing`，干净提交 `9243a79b847415b29480d6e418dfc76d01307fcd`，SHA256 `168b0689ba103ecfa51e444c97226dc22b2857a553b75bca4c2ba34f7ac17a81`；来源记录旁置 `.build.json`。Runner 自有 `billingacceptance TestUsageSteerLineFixtureHandoff` 在新 `runner-fixture` 准备了两条历史形式夹具（无配置归属），只用于证明公开读取及未分配处理，不证明原生模型或新归属采集。

| 项目 | 命令/范围 | 通过条件 | 当前状态 |
| --- | --- | --- | --- |
| CB01 | 定向 test-feedback：billing、runnerclient、app 的 TestCalculate/TestRejectAmbiguous/TestRunUsage | 多模型/阶段分组、精确算术、时段边界/供应商歧义、未知与部分、观测缺失/别名/冲突不阻断、配置缺失不猜、固定快照分页 | 通过；`diagnostics/owner-main/summary.json`，37项、零失败/skip，48.005秒 |
| CB02 | 固定 `-level=L1`、`-level=L2` | 正式集完整通过、无意外 skip、清理核实 | 通过；`test-results/L1/20261109-main/summary.json` 与 `test-results/L2/20261109-main/summary.json`；L2 cleanup_complete=true |
| CB03 | `scripts/test-web.ps1 -EvidenceDir <scope>/test-results/L4/web-http-main` | 生产前端构建、web.unit 与 go.http 完整通过；分组展示、缺口提示、设置字段语义及现有缓存隔离 | 通过；该目录 `build.log`、`web-unit/summary.json`（354项）、`go-http/summary.json`（七包138项），全部零失败/skip |
| CB04 | 新独立 Runner 构建、`-level=L4 -suite=go -scenario go.runner-contract` 及定向 `-tags=runneracceptance -run ^TestBillingUsagePublishedFixture$` | 固定新提交/version/SHA；能力解码和公开 CLI 边界真实匹配；不以旧构建或合成原生证据代替 | 通过；`test-results/L4/runner-contract-main/summary.json` 三组共5项、零失败/skip；`diagnostics/published-usage-main/summary.json` 1项通过；`runner-fixture/cleanup.json` verified=true |
| CB05 | 文档链接、git diff --check、runneracceptance 编译 | 当前文档无矛盾/坏链接，所有受影响标签测试可编译 | 文档和差异检查通过，证据 `diagnostics/doc-final`；app 全部 runneracceptance 测试随 CB04 编译通过 |

自查覆盖旧模型/时间门槛删除、stage/model/effort分组、价表歧义、当前profile不倒填、页面迟到/缓存隔离及公开能力边界，未发现待修问题。新归属的原生采集仍由 Runner #133 自有验收负责；本次公开 CLI 夹具只证明缺归属不会被观测模型补齐，配置模型正向计价由 CB01 的公开负载子进程夹具和精确算术验证。

本次不重复真实模型或远端采集验收，不修改 Runner 仓库，不承诺历史候选一定能补齐配置归属；Runner 返回缺项时如实展示。未运行真实浏览器，本次 UI 自动验证为组件全集，已有 browser smoke 仅同步字段名称。无数据库变化，不选择 go.storage。用户后续已授权提交、推送并创建 PR；未部署安装版。
