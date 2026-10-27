# PR399 冲突解决与合入审计

2026-10-27，用户要求解决 [PR399](https://github.com/erranliu/SteerLine/pull/399) 冲突并审计是否适合合入。审查针对进度修复增量，不将新主分支的 roadmap、部署与远端环境功能重新算作本 PR 新增功能。

## 结论

冲突已解决，代码审查未发现阻塞合入的缺陷；建议保持草稿，暂缓立即合入，等待原约定的连续两次真实进度链验收。组件和受控联验通过不替代该验收，也不证明主线新环境适配/result-file 代际已完成真实远端联验。

受控进度场景首轮在维护接口读取时超时，未进入进度断言；独立 main 对照及 PR 独立复核通过，尚未稳定复现，也未证明原因已消除。保留这项验证风险，不能称作已修复的主分支缺陷或覆盖原失败记录。没有自动合并、部署或更换在途轮次二进制。

## 冲突与代码范围

- 合入目标 `main` 为 `02a8e1e77ff4e14840e81943180e0590b33b28f9`，本地冲突解决提交/实际受测来源为 `ed03cc6a8076edde14f4927e309e8bc36796481f`；后续仅审计文档提交另记，不冒充在文档 head 重跑代码测试。
- 唯一冲突是 `internal/app/http_consumer_boundary_test.go`。采用 main 的 Web 宿主限定例外，未采用 PR 原先面向所有消费者的较宽例外；该文件与 main 无差异。其余依赖闭包仍逐包检查，未允许数据库或执行所有者进入 HTTP 消费端。
- 审查实施/审计执行与发布/报告的提示组装、用户 override 保留、同源 PowerShell 写入器嵌入、必跑测试与 Plan 定向测试的生成优先级。调用方仍验证 Runner 公开配置 schema，冻结安排、严格布尔字段解析和原 Run/输入身份未放宽；无业务测试 ID 硬编码、旧版读取或回退。
- 写入器实际接入项目登记入口，由原结果、输入与清理核验决定布尔值。发布/报告收到只读提示；这属于步骤指令约束，不是新增 OS 文件权限隔离。错误字段仍由原严格解析拒绝，不能仅凭提示或模型回复认定真实链成功。
- `projects_test.go` 维护既有 restart 启动器断言，与现有实现一致；没有在本 PR 新增重启功能或变更安装流程。

## 验证

证据根 `C:/SteerlineDebug/wt-507e-pr399-review/`，各次原报告及失败均保持独立。

| 范围 | 实际结果 | 原证据 |
| --- | --- | --- |
| 固定 L1 | 通过，干净受测来源 ed03cc6a | `L1-20261027/summary.json` |
| 固定 L2 | 通过，cleanup_complete=true；受控 worker/provider，无模型请求 | `L2-20261027/summary.json` |
| 直接受影响 app | HTTP 边界、restart 启动器、固定测试快照三项全部通过，无 skip | `app-affected-20261027/summary.json` |
| 三个受影响 owner 包 | runnerclient、testmanagement、testmanagement/model 均完成无失败；显式原生 opt-in 未选入，不算原生验证通过 | `owners-20261027/summary.json` 及原 JSONL |
| 较广 app 运行 | 总默认10分钟预算耗尽时处于新主分支 roadmap 实际 Git 用例，整体 incomplete；不是全 app 通过 | `owners-20261027/summary.json` |
| 进度 L3 首轮 | 维护接口 GET 超时，失败，清理通过；未进入进度断言 | `progress-L3-20261027/summary.json` |
| 未修改 main 对照 | 同一场景通过，来源02a8e1e7，清理通过 | `main-progress-L3-20261027/summary.json` |
| PR 独立 L3 复核 | 同一来源 ed03cc6a，通过/重跑清零/非法内容保留/断连恢复/重启身份与零额外输入/closed保留/清理全部通过 | `progress-L3-isolated-20261027/summary.json` |

第一次 L3 命令被 PowerShell 参数解析拒绝，发生于测试执行前；改用显式 argv 数组后才执行首轮。拒绝记录为 `L3-command-rejection.json`，不作为产品失败或通过。

原生 opt-in 未执行项：

- `TestEnvironmentCheckPublishedContract`
- `TestProjectEnvironmentExecutionPublishedResultRoundTrip`
- `TestRunnerFullBindingPublicCLI`

`audit-result.json` 保存上述报告 SHA 与审计来源。未运行业务项目全量测试；业务断言及夹具修复由用户合并的 PR428 交付，不以此审计替代其完整业务/人工验收。

## 原进度复验仍在继续

PR428 已确认合入 AIRefactor `485d88bad9081d3fd8d892116569f763038f7b7e`。用户随后恢复原进度复验，新 `post428-01` 已接纳实际10项安排；历史 qualified-01/02 不重放、不重新计数。新轮仍使用已批准的原 Caller8205/Runner7c/SDK 子环境，和本次受测 main 集成 ed03cc6a 分开记录；它不是当前 main 新环境代际的替代证明。

截至本次审计没有连续2/2通过结论。原报告、进度/API/UI、真实发布与原 Run/宿主/原生资源清理仍须逐轮独立核实。详细历史见[原进度修复复验](test-progress-0of6-20261026.md)、[Spec](../../spec/runtime-test-progress.spec.md)和[Plan](../../plans/runtime-test-progress.plan.md)。
