# L2/L3 测试依赖边界实施

基于未合并的 #180 建立独立分支与 PR，只包含本次解耦改动。保持同仓、同 go.mod；不修改生产行为和生产合同。

1. 新增测试自有 evidence 层，集中 profile/resource/session/Orca codec 验证，输出小型证据 DTO 和统一校验。
2. 原生可执行文件、配置 codec key、Orca 返回与库存解码集中到 native；查询 reader 调用集中到 driver。testenv/场景仅组织现场和断言。
3. 新增 L1 依赖边界测试并加入 quickPackages。生产检查基于真实 Go 依赖图排除作为检查入口的 fixture 包；E2E 检查启用 e2e tag，限制实现包只能被 driver/native/evidence 导入。合成图验证传递违规，防止“目录同仓”被误判为生产依赖。
4. 更新 E2E README，执行以下最小验证，记录证据，再 commit/push 并创建基于 #180 分支的独立 PR。

## 测试计划

| 级别 | 组/场景 ID | 环境 | 命令 | 时点与通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | dependency.production / dependency.e2e-layers / evidence.identity | 本机，无模型 | `go test ./internal/testboundary ./tests/e2e/internal/... ./cmd/test-feedback -count=1` | 抽取后；实际依赖图通过，传递违规拒绝，模型/权限/原会话与回合证据保持，错误身份拒绝 | validation 目录命令记录 |
| L1 | 固定最小入口 | 本机，无模型 | `go run ./cmd/test-feedback -level=L1 -out=<fresh>` | 接入后；新边界包在 quick-rules 中实际运行，完整最小入口通过 | summary.json |
| L2 | smoke.delivery | controlled，无模型 | `go run ./cmd/test-feedback -level=L2 -env=controlled -out=<fresh>` | 定向通过后；交付、身份读取、清理通过 | summary/scenario |
| L3 | recovery.waiting-merge-restart | controlled，无模型 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -out=<fresh>` | 同上；恢复原会话/权限、交付、清理通过 | summary/scenario |
| 编译 | 全部 E2E 场景 | 本机，不运行场景 | `go test -tags=e2e ./tests/e2e/live ./tests/e2e/smoke ./tests/e2e/suites/... -run '^$' -count=1` | 抽取后；真实/受控场景编译通过 | 命令输出 |

证据目录：`docs/validation/e2e-dependency-boundaries/`。不选 L2 live、L3 live 和 full：生产执行与原生操作语义不变，抽取使用原 codec，证据单测覆盖两条原生链路；本次验证无需额外 token、测试 PR 或全量基线。
