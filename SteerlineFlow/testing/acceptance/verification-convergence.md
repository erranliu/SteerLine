# 核验规则收敛：改动与验证

2026-11-10。普通开发，用户先授权收敛测试时点、按行为选择测试及证据停止条件，随后授权实施独立审核复核、接线检查触发条件和证据复用的文档澄清。调查及验证基线为 `4f1a4bcb285fe224d524f9ca1e23b0f6bdb12290` 加本工作区未提交变更，不冒称干净提交通过。规则实施及验证阶段未启动 roadmap、真实模型或外部 PR；后续 PR 交付见文末，不合并、不部署。

## 改动与业务原因

- 测试规范、项目指南及相关入口统一为一批代码完成、候选稳定后运行一次固定回归，后续按实际行为影响补测；局部展示不自动触发 Web/后端 owner 全集。取证保留最小摘要、代表截图与原报告定位，必需验证和问题闭环完成后停止。
- roadmap 首次仍由独立只读 reviewer 完整审核；未参与写入的原 reviewer 可复核原问题、新增差异及受影响路径。范围扩大、重新组织实现或影响无法收窄时扩大审核；原 reviewer 不再独立、不可用或用户明确要求新视角时更换。保持 reviewer/fixer 分离及唯一写入者。
- 固定测试材料将每任务逐 ID 源码检查改为首次接入、测试入口或进度逻辑变更、实际进度异常时按受影响 ID 检查。其它任务直接执行固定登记入口；缺口报告具体入口和原因，按任务范围修复，扩大范围时待决。保留真实执行、开始清零、完整成功才更新、禁止旧报告回填及发布/报告只读。
- head/base 变化仍使当前结论失效并保存历史；重新评估差异后，当前候选的审核及验证汇总可引用未受影响的旧报告，保留原 SHA、适用范围与原因，新增差异单独补测。合并门禁仍重新查询平台；不改状态脚本、记录结构或新增缓存。
- 同步技能入口、协调规则、交接/复核/heartbeat/PR 模板、初始化指南、运行时进度说明及当前 Spec 契约。初始化指南中缺 writer 时内联提供源码的旧描述改为现有代码实际执行的投递前阻断。历史验收和原请求不追改，新材料仅作用于新请求。

## 正文差异与 UTF-8 字节

完整增删差异见[实际指令与材料 diff](evidence/verification-convergence-prompts.diff)（`--unified=0`，省略未改上下文并修剪 hunk 头的尾空白），不是通过更新期望值代替审核。下表按 UTF-8/LF 比较 Git 基线和本次正文：

| 文件 | 修改前 | 修改后 |
| --- | ---: | ---: |
| `internal/prompts/materials/testing.md` | 3256 | 3333 |
| `SteerlineFlow/testing/runtime-progress.md`（同源副本） | 3256 | 3333 |
| `.agents/skills/roadmap-runner/SKILL.md` | 11796 | 12243 |
| `.agents/skills/roadmap-runner/references/coordination.md` | 18386 | 19839 |
| `.agents/skills/roadmap-runner/references/prompts.md` | 8062 | 9398 |

材料增长 77 字节，用条件触发与缺口处理替代无条件逐项查接线；技能增长用于说明原 reviewer 复用、增量复核范围及旧报告归属，保留合并保护。测试材料与项目副本归一化后相同，新资产摘要为 `8ec76ded720a6d01623da9e70c48f6b24795344a82018fd3f59895f6b5154421`；仅更新 `TestDocumentDrivenRuleCopies` 对应资产摘要。短指令正文及黄金样本未改，`testing-reference` 仍为 95 字节、`testing-step` 样本仍为 123 字节。writer、工作流绑定、状态脚本及 CI 未改。

## 验证与审核

本轮证据根：`C:\SteerlineDebug\wt-0cba-verification-convergence`。固定回归按本次治理要求执行；原始报告保留独立运行目录。下表路径均相对此根，受测来源为上述 HEAD 加未提交变更；测试后仅补本记录与索引，不改受测指令或测试逻辑。

| 检查 | 实际结果 |
| --- | --- |
| `go test ./internal/prompts -count=1 -v` | 通过；短正文黄金样本、固定材料摘要及同源边界通过，实际输出见本聊天工具记录 |
| `python .agents/skills/roadmap-runner/scripts/test_state.py -v` | 18 项通过，`diagnostics/state-tests.log`；验证现有候选失效、历史与合并门禁，不创建真实外部资源 |
| skill-creator `quick_validate.py` | 通过，`diagnostics/skill-validation.log`；以 `PYTHONUTF8=1` 读取中文技能 |
| 正式 L1 | `go run ./cmd/test-feedback -level=L1 -out <本行目录>`：退出码 0、passed=true、managed l1.minimum passed；`test-results/L1/20261110T073704581Z-13c5178d28ef4863abc016797c831e32/summary.json` |
| 正式 L2 | `go run ./cmd/test-feedback -level=L2 -out <本行目录>`：退出码 0、passed=true、managed smoke.delivery passed，原 summary 与 scenario 的 cleanup_complete=true；`test-results/L2/20261110T073743952Z-054a07716b23411781be1866dab0cc00/summary.json` |
| 文档链接、正文/副本/引用冲突及 `git diff --check` | 737 份 Markdown、4576 个本地链接、0 错误；diff 空白检查通过。正文副本及完整 diff 一致，无现行指令冲突；状态脚本/writer/工作流/黄金样本/CI 未改 |
| 独立审核 | `/root/review_verification_rules` 独立只读审核无未解决发现，明确批准本次固定提示词/规则资产、技能规则及对应摘要期望更新；原报告见本聊天审核消息。核对完整正文、5份字节账本、同源副本、短样本、生产固定文件读取与投递前阻断、旧请求冻结及合并门禁，未写文件或代填测试通过 |

首次沙箱内 npm 安装以 `Exit handler never called` 退出；状态测试因沙箱临时目录权限失败，技能校验因默认 GBK 解码失败，原失败见本聊天工具输出。未把这些计为通过；后续使用本任务隔离目录和 UTF-8 完成状态/技能复验，未修改脚本或降低断言。锁定依赖随后安装成功（160 packages，`diagnostics/npm-ci.log`），正式 L1/L2 如上通过。不将这些受控/离线验证表述为真实 roadmap 调度、模型行为或远端 PR 链通过。

## PR 交付准备

用户随后授权将两轮修改合为一个提交、推送并创建 PR，暂不合并、不部署。暂存检查发现原证据 diff 的空上下文行被判尾空白，现改为零上下文 diff，保留全部实际增删及规则正文；此前未跟踪文件未包含在普通 `git diff --check` 中，交付改用包含新文件的暂存检查。已有测试结果保留原受测版本，不因提交或证据格式调整重跑。

交付分支为 `codex/verification-rule-convergence`，整合 main `1f64b0ff`。唯一冲突为验收索引顶部插入，保留本次与上游 SteerAgent 两条记录；L4 说明自动整合并保留上游场景说明。五份已审核指令/材料与原提交 `2d1577a8` 归一化后相同，独立审核批准及本节原基线测试结果按原归属复用；未声称 L1/L2 在新基线重跑。交付只补文档及暂存/链接检查，不改变状态脚本、writer、工作流绑定或合并门禁。
