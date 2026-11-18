# 安装级重构方向

「管理重构方向」提供安装级模板的新增、改名、正文编辑和删除。新建任务时可覆盖模板、另存为新模板或直接以当前正文启动。任务受理时保存名称和实际正文，模板后续变化不会影响任务。旧方向无需迁移。

## 提示词差异

PR 基于 `4510db60e1f13c87835cd7cb23485c59cdcad91d` 已合入的阶段决策模板和固定收尾流程。

| 材料 | UTF-8 字节数（LF） | 业务原因 |
| --- | ---: | --- |
| [决策模板](../../../internal/prompts/maintenance-decision.md) | 1802 → 1814 | 明确以本次方向正文判断，移除补充要求的引用 |
| [更新过期文档](../../../internal/prompts/materials/refactoring-directions/update-stale-docs.md) | 0 → 313 | 交付唯一内置方向，使其聚焦有证据支持的文档更新 |
| [旧内置方向](refactoring-directions.md#提示词差异) | 56、59、47、68、68 → 各 0 | 新安装只提供一个内置方向 |

决策模板和各阶段渲染正文的实际替换

```diff
-按维护方向和补充要求判断，默认保持现有行为。
+根据 direction_prompt 中的重构方向判断，默认保持现有行为。
```

首轮渲染正文 604 → 616 字节，后续选题 592 → 604，成果审计 598 → 610，PR 评审 379 → 391，PR 评审 JSON 重试 464 → 476。首轮、后续决策和 PR 评审的 `maintenance/context.json` 移除 `"requirements":""`，含相邻逗号时同一份空补充要求的样本少 18 字节。非空样本额外少原值的 JSON 编码字节数。`direction_prompt` 交付本次编辑提交的原正文。固定收尾和执行指令模板沿用主分支。

## 验证

以下历史验证基于 `db4615a3740f64b2324df9034c52051bd0bd8ab0` 加上方向管理改动。主分支更新后的复验另列。

- Go 提示词资产、黄金样本与实际 Runner 文件请求检查通过。实际请求测试覆盖首轮、后续审计和 PR 评审，核对原正文和已移除的字段。
- 重构方向定向 Go 测试覆盖安装级持久化、重名、旧版本、删除、当次正文冻结及原请求重放。重构前端 15 项组件测试及生产构建通过。
- L1 最小集通过。首轮发现 Web 代理引用了领域类型，改为 HTTP 契约类型后通过。报告 `C:\Users\lmas1\.codex\visualizations\2026\11\17\01a1e837-6f62-7f83-9bf1-515bd3da7351\L1-directions-f80b0291afa448e78afd5a95d2b59381\summary.json`。
- L2 受控交付冒烟通过。报告 `C:\SteerlineDebug\wt-afe9-directions\test-results\L2\f15e609ef6ea48cbbdd6b1e2f23f9659\summary.json`。
- L3 `runtime.refactoring-maintenance` 通过。Edge 验证方向增改删和重名拒绝、桌面和窄屏正文编辑及直接启动。受控 Runner 验证模板更新后任务继续使用冻结正文，并完成原两轮交付、评审、并发排队和清理断言。报告 `C:\SteerlineDebug\wt-afe9-directions\test-results\L3\a0d11d92f5e248a9b924cc5a64ccb898\summary.json`。
- L4 `go.storage` 复验中 Windows 路径检查通过，SQLite 的环境活动摘要和活动分页请求关联两项断言失败。环境活动摘要的同一断言在未改动基线也可复现。活动分页在本次两次运行中的结果不同。报告 `C:\SteerlineDebug\wt-afe9-directions\test-results\L4\storage-a4bec6728dab48a0898f22d6bb223c9f\summary.json`。
- L4 `go.http` 通过。报告 `C:\SteerlineDebug\wt-afe9-directions\test-results\L4\http-89e7d502484d48338ee346a7a639332a\summary.json`。

### 活动分页归因

存储专项失败的 `TestAsyncActivityPaginationAndRequestLookup` 只写入 520 条项目基线检查命令。原基线 `db4615a3`、合入最新主分支的版本以及本 PR 的基线检查查询和数据源时限实现一致。

用同样的 520 条命令在旧基线和当前版本各做两轮诊断。测试夹具合并为一次写事务，分页读取和断言保持原逻辑。正常读取均返回 520 条记录及对应请求关联。让基线检查摘要占满已有的 1 秒数据源预算后，两版均报告 `links: context deadline exceeded` 并缺少请求关联，单独调用关联查询均能返回该项。原存储专项的失败日志只有断言，无法确定当时触发时限的具体阶段。此 PR 未修改分页和请求关联逻辑。

诊断记录位于 `C:\SteerlineDebug\wt-afe9-directions\diagnostics\pagination-997aa35a`，两版输出分别为 `baseline-probe.log` 和 `current-probe.log`。本 PR 同时在重构活动摘要中剔除新增的方向正文，定向测试通过。

### 更新主分支后复验

基线为 `4510db60`。15 项重构前端测试、生产构建、文档链接检查、SQLite 重构摘要和方向持久化定向测试均通过。独立复审批准更新后的提示词差异，逐项核对阶段指令、固定资产和实际 Runner 文件请求，定向测试通过。

- L1 通过。报告 `C:\Users\lmas1\.codex\visualizations\2026\11\17\01a1e837-6f62-7f83-9bf1-515bd3da7351\L1-rebase-03fc3cecac694c8a87dac3f6461e607c\summary.json`。
- L2 通过。报告 `C:\SteerlineDebug\wt-afe9-directions\test-results\L2\rebase-7c935b0cbd4d420d9e1ceb4bd6680671\summary.json`。
- L3 `runtime.refactoring-maintenance` 通过。报告 `C:\SteerlineDebug\wt-afe9-directions\test-results\L3\rebase-9d860296b7bf42b3b7b3c8cb38973854\summary.json`。
