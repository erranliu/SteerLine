# Plan：三窗主界面操作与控制台可读性优化

绑定 [Spec](console-primary-workspace.spec.md)，实施基线 `172cea6c6530b6693b742324a9c1d4f9aeb17b6d`。复用当前工作区已审计 Spec 与第一批三窗实现；不修改持久化/数据库/产品协议版本，不引入兼容路径。

## 实现步骤

1. 用同一布局计算三窗边框、概要和右下操作空间，语义颜色适配终端能力；选中与焦点分别表示。右上优先目标、冻结方案、状态、阻断及下一步，左侧补候选汇总待处理提示；底部使用当前模式快捷键。
2. 方案选择、候选放弃确认在右下渲染；操作锁定原目标/版本/方案摘要。Tab 保留模式并返回列表，切任务/项目/候选取消临时操作；统一同屏核对门槛，Enter 显式确认，移除 y 确认旁路。停服使用同一可见性核验。
3. 普通 Enter 到右下或激活回答；保留 Unicode 编辑与原请求提交。详情/F8 独占按键，F8 返回保留模式/焦点；编辑切模式先保存，迟到拒绝不自动打开编辑器。
4. u 在右下呈现原只读核验摘要，r 按焦点重读报告/项目，过期报告标记。规范 final 默认仅读取有界前缀，f/[ / ] 时读取完整正文；当前动态不被旧 final/终态回执遮住，不从通用 Summary 及 worker 自报制造已确认结果。
5. 更新受影响断言与操作说明，执行最小验证、自查并保存证据。一个独立实现 PR 包含 Spec、Plan、实现与验证记录。

## 测试计划

证据根目录 `docs/validation/console-primary-workspace/`；runner 每次使用新子目录，保留失败结果。

| 级别、组/场景 | 环境与命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| 定向 CPW-01–04，console 全包及读取消费者 | Windows、临时 home、公开 DTO fixture；`go run ./cmd/test-feedback -out=<新目录> -- ./internal/console ./internal/product/query ./internal/product/client -count=1` | 新交互/读取接通后 | 新增 `TestCPW*` 与受影响现有用例通过，零匹配/skip 不作通过；原目标、版本、草稿与请求边界保持 | `targeted-*/summary.json`、Go JSON/stderr |
| 定向接线 CPW-02/03 | Windows、临时 SQLite、受控 worker；`go run ./cmd/test-feedback -out=<新目录> -- ./tests/integration -run '^TestPS1InlineConsoleWiring$' -count=1` | console 回归通过后 | 真实 app 客户端原地反馈仍绑定同一候选，保存/送达/续轮事实正确 | `integration-*/summary.json`、日志 |
| L1 固定最小组 | Windows；`go run ./cmd/test-feedback -level=L1 -out=<新目录>` | 定向通过后 | passed/complete 为 true，规则与依赖边界保持 | `l1-*/summary.json`、各组日志 |
| CPW-05 原生终端 | Windows ConPTY、隔离 fixture，无 worker/模型/PR；`go test -c -tags=ps1terminal -o <binary> ./internal/console`，绑定 `STEERLINE_CPW_TERMINAL_CAPTURE` 后运行 `-test.run=^TestCPWWindowsTerminalInput$` | 确定性回归通过后 | 实际 Bubble Tea/VT 输入、中文多行、Tab/Enter/F2/Esc/F6/F7、F8 返回、宽窄焦点正确，绑定原候选且测试 PASS | `terminal/capture.json`、原始 VT 输出；不证明物理 IME |
| 静态、构建、文档 | `go vet ./internal/console`；`go build -o <binary> ./cmd/steerline`；`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 交付前 | 无错误，帮助/文档与 Spec 一致 | 验证 README 与命令日志 |

不选 Full、L2、L3/live：本任务不改原生交付、恢复协议、provider 或资源清理；console 确定性回归、真实 app 受控接线及一次 ConPTY 足以验证本次边界。不新增 E2E 注册场景或生产测试能力；只新增 console fixture 的行为断言及显式 `ps1terminal` 终端用例，默认测试不含 opt-in skip。

## 状态

实现、124 项发布定向回归、原地反馈接线与当前构建 Windows ConPTY 已完成；固定 L1 原集成组累计超时，报告不完整，补跑缺失项。具体限制及证据见[验证记录](../validation/console-primary-workspace/README.md)，不以本计划替代验收证据。

追加最小诊断：Windows 受控环境，`go run ./cmd/test-feedback -out=docs/validation/console-primary-workspace/l1-integration-tail -- ./tests/integration -run '^Test(TM04ExecutionWiring|TM06ResourceWiring|TM08AppHostWiring)$' -count=1`；在 L1 超时后执行，要求这些原场景全部通过、passed/complete=true，证据保存在该新目录。原 L1 的 passed=false 保持，不将多个报告拼成一次成功的 L1。
