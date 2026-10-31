# 执行方案与项目运行环境界面梳理：验收计划

2026-10-31。关联普通开发，无 Task ID。[Spec](../../spec/runtime-environment-ui.spec.md) · [Plan](../../plans/runtime-environment-ui.plan.md)。状态：已按用户后续要求审计并实施，界面自动项通过；历史固定 L2 两次失败；后续修复已补验通过，Windows 旧现场根因未定，见[后续定位](l2-receipt-reliability.md)。人工 REU-M01 待验收。不部署或操作安装现场。

## 1. 范围与准备

验收执行方案/运行环境的页面分工、五类事实及六阶段、卡片对象绑定、任务定向准备与返回、缓存故障与跨项目隔离。沿原项目×环境资格、普通 Run/result、未知请求和停止结算；不重新验证或授予三条原生路径 ready。

调查基线：`cb0c888e7ed6ccea3f9d5c9e2e6427cc3884d0a6`。本轮为该 SHA 上的未提交工作树，Spec/Plan 随同本次改动；Web 构建摘要与实际结果见第 5 节。原始输出按 [paths](../../paths.md) 使用仓库外独立 scope 和每次新目录；首轮文档检查与后续实现结果分别记录。

自动准备使用现有 Web fixture、临时受控 HTTP 服务器和既有命令用例；数据覆盖本机 Codex、本机 Orca、远端 Orca、同机多上下文/同环境多模型、两个项目及实例换代。它们只验证 UI/调用方，不冒充真实机器能力。实际入口/时点/通过条件以 Plan 第 3 节为执行安排。

## 2. 自动验收项

| ID | 方法和最小操作 | 最小通过条件 / 阻塞范围 | 状态与证据 |
| --- | --- | --- | --- |
| REU-A01 | Web owner + 受控浏览器：全局配置、项目配置、运行环境、原返回/后退、未保存草稿 | 入口同名且职责分明，原导航保护保持；失败阻塞界面交付 | 已验收（本轮界面范围）；见第 5 节 |
| REU-A02 | Web owner；必要时 app DTO owner：同机多环境、同键多方案、跨项目、同名目标、空列表/首次失败、敏感值 | 分组与原键一致、标题能辨认、生成方案不计数、不泄露凭据；失败阻塞界面交付 | 已验收（本轮界面范围）；见第 5 节 |
| REU-A03 | Web owner + 受控浏览器：五类事实、六阶段、显式失效且原活动未结算、PR 发布与合并分界 | 不改资格或状态优先级，原停止可用，不混淆模型可调用及任务可派发；失败阻塞交付 | 已验收（本轮界面范围）；见第 5 节 |
| REU-A04 | Web owner + 受控浏览器：成功后 options 失败、离线、恢复、正常轮询、项目/实例切换，独立活动详情可读 | 缓存标待核实并有观察时间；新操作不借旧 ready，原停止不被 options 故障误封；失败阻塞交付 | 已验收（本轮界面范围）；见第 5 节 |
| REU-A05 | Web owner + 原命令回归：材料生成/导出、卡片内多方案/无默认、超时、双击、占用拒绝、未知回执 | 材料不授予资格；参数精确绑定卡片；不暗选方案、不重发未知命令；失败阻塞交付 | 已验收（本轮界面范围）；见第 5 节 |
| REU-A06 | Web owner + 受控浏览器：失效/停止分别校验版本，旧未结算活动、列表外报告、当前与历史通过、结果摘要和折叠原文 | current/activity revision 不混用；旧报告不授予资格；原活动可定位可停止，PR 只表达已核实事实；失败阻塞交付 | 已验收（本轮界面范围）；见第 5 节 |
| REU-A07 | Web owner + 受控浏览器：任务/默认环境定位、URL 刷新、返回 task/方案，删除方案/无效 ID、用户暂停 | 原上下文恢复或明确报变化，零导航副作用、零自动换方案/改派发；失败阻塞交付 | 已验收（本轮界面范围）；见第 5 节 |
| REU-A08 | 扩展原 configuration-navigation 浏览器：主流程、故障缓存、双项目、键盘、宽窄屏与长内容 | 核心功能有自动断言，状态不只靠颜色，卡片/按钮可达且页面无整体横向溢出；失败阻塞交付 | 已验收（受控浏览器）；见第 5 节 |

必要回归：固定 L1 和完整 Web 专项通过；界面轮次的固定 L2 两次失败及独立基线通过分别保留。后续修复已通过固定 L1/L2，见[后续定位](l2-receipt-reliability.md)；Windows 原始错误仍未定因。原门禁规则未改，不重复原 readiness L3；命令、日志、非零用例数、退出码和清理结果按每次实际运行登记。受控 UI/API 不代表真实 Runner、模型或远端验收。

## 3. 后置人工体验

REU-M01，方法：人工。参与人、窗口和受测构建待安排，可在自动验收通过后集中进行，不阻塞无关后续工作。状态：待验收。

1. 从任务不能启动进入对应环境，说明运行机、执行环境和模型方案的关系，找到具体阻断原因及下一步。
2. 从材料区找到准备要求，在卡片内确认方案及覆盖范围；不实际调用真实模型或创建 PR也能评估操作对象是否清楚。真实动作另需隔离现场和原授权。
3. 查看验收摘要及历史报告，分清当前资格与历史通过；返回原任务并核对所选方案。
4. 检查窄屏、长名称和键盘操作的可理解性。自动已知失败或工程前置缺失不能列为该人工体验项通过。

最小通过条件：无需口头解释即可完成上述辨认和导航，能正确回答“为什么不能启动、下一步是什么”。实际反馈及范围单独登记，人工确认不能替代 A01–A08。

## 4. 首轮文档检查（实施前）

仅检查 Spec/Plan/验收互链、ID 与步骤映射、现行 ready/生命周期约束、UTF-8 和 diff 空白；不运行代码测试。实际命令与结果完成后登记于此，不能记为产品自动验收通过。

2026-10-31，首次文档交付时的静态核对（后续已按用户新要求实施）：

- `python scripts/check_doc_links.py`：626 个 Markdown 文件、3953 个本地导航链接，0 错误，退出码 0。
- `git diff --check`：通过，退出码 0；新增三份文档另以 Python 严格 UTF-8 解码并逐行检查尾随空白，通过。
- 人工核对 Spec/Plan 的步骤与 REU-A01–A08、REU-M01 映射及现行发布/停止边界；Spec 与验收记录的全部 ID 另以脚本检查完整。
- 工作区仅新增 Spec、Plan、专属验收计划并更新两处 Markdown 索引；无代码、配置、提示词或测试资产变更。未运行 L1/L2、Web、浏览器或真实环境测试，产品自动/人工验收状态不变。


## 5. 审计、实施与实际验证（2026-10-31）

### 范围审计

本聊天自审后收窄：profile/task 足以定位与返回，删除 URL 中重复的 environment/run 和原候选恢复；不新增选择存储、权限/命令行差异分析或动作状态机；不为展示另查远端目录。Spec 第 9 节记录决定，Plan 同步实际测试安排。本轮没有修改模型提示词、环境执行材料、相应黄金样本或 CI。

### 实施结果

- 全局执行方案按 target 分组，项目导航及配置快捷入口统一为“运行环境”，目录按钮改为“刷新能力目录”；保留原保存和加载规则。
- 项目准备文档只显示一次；环境卡片显示链路/Agent/短键、共用方案、原因和下一步；开始、超时、方案选择及失效原因绑定同一卡片。
- options 只补原 target/adapter/agent、activity_settled 和从原文档 manifest 读取的 script_path；不发送完整 settings、不更改键/资格/数据库/Runner。
- 查询失败的缓存保留“上次已知状态，当前待核实”和观察时间；禁用依赖当前观察的新操作，原历史活动独立查看和停止。
- 任务/默认方案链接带 profile，任务来源另带 task；支持刷新定位、返回原任务并恢复方案。无效方案明确提示，不自动执行。
- 当前及列表外报告按原 ID 聚焦，检查/L2/发布/收尾优先，原文件折叠；旧未结算活动的停止保持原 ID/revision。

### 证据与结果

证据根：`C:\SteerlineDebug\wt-5e65-runtime-env-ui`。所有结果均为调查 SHA 上的未提交实现，不能写入干净提交的测试历史。既有正文文档检查不计为产品通过。

| 范围 | 实际执行 / 结果 | 原始证据 |
| --- | --- | --- |
| 原环境分组 owner | `go run ./cmd/test-feedback -out <owner目录> -- ./internal/app -run TestProjectEnvironmentReadinessLocalAndModelGrouping -count=1`；原用例非零命中并通过；扩展核实展示字段、脚本路径和 settings 不外泄 | `diagnostics/owner-20261031-02/summary.json`；首轮原用例结果保留于 `owner-20261031-01` |
| 固定 L1 | `go run ./cmd/test-feedback -level=L1 -out <L1目录>`；通过 | `test-results/L1/20261031-01/summary.json` |
| Web 全集 | `pwsh -NoProfile -File scripts/test-web.ps1 -EvidenceDir <Web目录>`；生产构建、31 文件/225 测试、HTTP/server owner 全通过，包装器退出码 0 | `diagnostics/web-20261031-02/`；server 原摘要为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261031T083811.021644800Z/summary.json` |
| Web 定向补测 | 增加“共用环境无默认方案不得任选、删除的定向方案不替换”反例后，`npm --prefix web test -- src/features/environment-checks/project-environment.test.tsx`；13/13 通过；首轮两处旧界面断言失败已修订并通过 | Codex 本轮命令输出；最终全集结果见下表收口项 |
| 受控浏览器 | `npm --prefix web run test:e2e -- configuration-navigation.spec.ts`，Edge、单 worker、零 retries；2/2 通过；含六状态、任务往返/刷新、缓存失败恢复、双项目、键盘、1440/375 宽度。截图已目视核对，无整体横向溢出 | `diagnostics/browser-20261031-02.log` 及同名目录截图/trace；最终构建 2/2 通过另记 `browser-20261031-03.log`，并保存同名目录截图/trace |
| Web PS5 包装器原失败 | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-web.ps1 ...` 退出 1；build、225 客户端用例和底层 server 测试均通过，但 PS5 把 stderr 上的成功摘要当 NativeCommandError；不将该包装器记通过，不修改无关脚本 | `diagnostics/web-20261031-01/`；底层 server 摘要 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261031T083551.022567900Z/summary.json` |
| 固定 L2 首轮 | `go run ./cmd/test-feedback -level=L2 -out <L2首轮目录>`；失败。自动材料观察显示 unknown / request_not_found，maintenance GET 等待 5 分钟后超时；scenario 的 cleanup_complete=true；未进入任务交付主断言 | `test-results/L2/20261031-01/summary.json`、`scenarios/smoke.delivery/delivery-maintenance/automatic-material-latest.json`、`scenario.json` |
| 干净主干对照 | 独立 detached checkout `baseline-source` 固定 cb0c888e，`git status --short` 为空；同一 L2 通过，TestDeliverySmoke 117.18 秒。因此不能将首轮失败归因主干缺陷，也不能据此证明本次实现通过 | `test-results/L2/baseline-20261031-01/summary.json`、原场景及 frozen-binaries |
| 当前版本 L2 补测 | 保留原失败，检查失败阶段和对照后单独补测。TestDeliverySmoke 74.67 秒失败于 start_task 的 request_receipt_persist_failed：原回执 .tmp rename 到 .json 报 Windows Access is denied；cleanup_complete=true；不记通过 | `test-results/L2/20261031-02/` |

两次 L2 错误不同，均保留原日志；干净基线通过，不能据此断言是基线缺陷，也不将补测失败或清理完成记为交付通过。本轮未修改材料生成和命令回执文件机制；根因未最终归因，整体回归仍有缺口。没有为通过测试改自动材料、提示词、重试或超时。REU-M01 仍待安排；没有真实模型、PR、安装或远端机器操作。


最终 Web 生产构建：`web/server/dist/assets/index-D8GIj5vh.js`，SHA256 `727235c66ad31eeef8c6d28b8e6364b4473c84c7b3a70bb59606e3d769a30ce5`。截图和 trace 均来自受控页面，不能代替 REU-M01 人工体验签收。


最终收口补充：`npm --prefix web test` 在最终代码上 31 个文件、226/226 用例通过，日志 `diagnostics/client-final-20261031.log`；最终生产构建与浏览器 2/2 均通过。文档链接检查 626 文件/3953 本地链接/0 错误，`git diff --check` 通过。基线对照 checkout 已在核实绝对路径且干净后移除，主干 L2 的 frozen-binaries、summary 和场景证据仍保留。固定 L2 的两次失败不因此变为通过，REU-M01 仍待安排。


2026-10-31 后续请求：用户要求修复 L2 并先定位 Windows，新增[专属记录](l2-receipt-reliability.md)维护后续范围与证据。原请求对账、夹具读取错误及诊断保真修复后，固定 L1、L2 串行通过；此前失败保持原结果。旧 Windows Access denied 的占用者/权限条件仍未复现或追溯，不能宣称其根因已修复。
