# PR 1 Plan：统一观察层与读取一致性

依据：[Spec](pr-01-observation.spec.md)。依赖：无。

## 实施步骤

1. **补齐读取 DTO。** 在 `internal/steerline` 新增观察快照类型及纯读取函数，复用 `LoadRuntime`。把新鲜度计算与文件读取分离，注入时钟。最新主线已经把 viewer Source 的 Runs 改为 Snapshot；在该接口上增加观察元数据并同步 fakeSource，保留 TaskDetail、conversation/raw 视图和已完成的布局修复。
2. **提取日志 cursor 引擎。** 从 `watch.go` 拆出共享读取模块；提供按批读取、reset 和 diagnostic 结果。Windows 读取句柄允许共享删除，文件身份由平台函数返回；Linux 使用对应文件身份。只在完整换行处推进提交 offset。
3. **接入 normalizer 与源选择。** 把已有 codex/pi normalizer 接到共享引擎；实现 fallback、流式固定源和 viewer 切源重置规则。保持 stdout JSONL/raw 契约。
4. **替换 viewer 的读取调度。** 删除独立 Scanner 实现，使用 stable run_id 选择与日志代次。引入 in-flight 标记及请求编号，拒绝迟到消息；把翻页与 follow 状态分开。
5. **修正 watch 路由。** `cmd/steerline/main.go` 根据流式参数选择入口，避免 follow=false 被 TUI 吞掉。更新 `docs/activity.md` 和 README 的参数说明。

建议新增 `observation.go`、`journal_reader.go` 及对应测试；平台文件只承载文件身份/共享打开能力。类型仍放在 `steerline` 包，避免让核心服务反向依赖 viewer。

## 验证

- 用临时文件验证完整行、半行、CRLF、Unicode、seq 回退、截断、等长/更大替换、超长与坏行；这些测试断言最终事件身份和 cursor 行为。
- 用可控 fakeSource 让响应倒序到达，覆盖选中项排序、A→B→A 和同 run 新 attempt。
- 沿用 TestWatch 的 fixture 验证默认流、JSON、raw、fallback、follow=false；viewer 模型测试覆盖错误与过期状态。
- 执行 `go test ./internal/steerline/viewer`，以及观察/日志/watch 对应的定向测试。Windows 上增加一次真实 TTY 冒烟：运行中追加日志、翻页、切任务、退出。
- 本 PR 不运行真实 Beads claim 或 agent。若文件替换测试出现 Windows 共享冲突，先修复句柄共享规则，不能仅靠增加重试时间使测试通过。

## 完成条件

- O-A1 至 O-A9 均有自动化覆盖或明确的 TTY 冒烟记录。
- viewer 与 watch 只保留一套行/cursor 处理规则。
- 文档准确描述 watch 固定 run/attempt 的行为；runtime 样例无需迁移。

## 风险与回退

normalizer 有内部状态，不能在每一批重新构造；文件代次必须控制其生命周期。读取层不写业务状态，回退这一 PR 不涉及 runtime 数据回滚。
