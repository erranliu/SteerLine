# 重构任务剩余预算

任务详情顶部单独显示已设置的轮次、金额或时间余额。调查派发时扣除一轮，重试和人工答复沿用已开始的轮次。金额使用已回传费用，费用不完整时提示待结算。预算耗尽后完成本轮，再自动收尾。有可交付改动时创建 PR，没有有效改动时保留调查结论。

## 固定指令与请求数据

独立审核已复核扣减、队列取消和费用回传边界。固定模板正文无差异，UTF-8 字节数为决策 1814 → 1814、执行 675 → 675、发布 525 → 525、冲突续作 299 → 299。

调查派发时冻结本轮编号，首轮及同轮重试的 `maintenance/context.json` 由省略轮次变为包含以下成员，增加 10 个 UTF-8 字节。

```diff
+,"round":1
```

后续选题从上一轮编号改为当前轮编号，例如 `"round":1` → `"round":2`，两者均为 9 字节。编号跨十进位时增加 1 字节。调查、实施和审计由此使用同一逻辑轮次编号。

## 验证

受测版本为 `1a91dbd53fffee792da02ba14e4d2813a446ec34` 加本次工作树改动。前端重构组件 30 项测试和生产构建通过。前端用例覆盖排队余额、首轮预算耗尽、交付阶段提示及金额待结算。

- `go test ./internal/refactoring/... ./internal/product/query -count=1` 通过，覆盖调查派发扣轮、末轮重试、费用回传前后余额，以及预算耗尽后完成本轮并交付 PR。
- L1 在独立 Windows 进程中通过。报告位于 `C:\SteerlineDebug\wt-703a-refactoring-budget\test-results\L1\6a236f3ebf7e4973b5a9966a758edb1d\summary.json`。首次运行的 Windows 依赖边界检查达到 60 秒时限。
- L2 和 L3 在独立 Windows 进程中通过，场景清理完整。报告分别位于 `C:\SteerlineDebug\wt-703a-refactoring-budget\test-results\L2\eb987e43972b4bc8927410351227dca4\summary.json` 和 `C:\SteerlineDebug\wt-703a-refactoring-budget\test-results\L3\eb987e43972b4bc8927410351227dca4\summary.json`。L3 的 Edge 检查覆盖预算耗尽提示、原 PR 入口和 375 像素窄屏。首次沙箱运行的 L2 和 L3 均失败，独立进程复验确认通过。
- 交付验证使用受控 Runner 和 PR 平台夹具。此工作树尚未部署。
