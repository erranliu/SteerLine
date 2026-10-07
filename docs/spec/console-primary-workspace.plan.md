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

## PR #234 质量审计测试计划

2026-10-07 同步 `origin/main` 的验证产物清理。审计实现基线 `6e920356`，同步后的提交 `d6e36a3d`。原始记录保留在本地临时目录，仓库只保留最小摘要；既有证据固定到 `6e920356`，不改写旧运行结果。

| 级别/组与场景 | 环境、命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 定向 CPW-02/03/04，`TestCPWAudit*` | Windows 隔离 DTO/home；`go run ./cmd/test-feedback -out=<临时目录>/before -- ./internal/console -run '^TestCPWAudit' -count=1` | 修复前 | 实际复现模式残留、回执遮挡、预览失效及 F8 滚动；截断目标另以明确长 ID 验证 | 临时目录 before 原始报告；验收摘要记录失败名称 |
| 定向 console/query/client | Windows；`go run ./cmd/test-feedback -out=<临时目录>/after -- ./internal/console ./internal/product/query ./internal/product/client -count=1` | 修复后 | 新断言及既有身份/输入/完整正文回归通过，passed/complete=true | 临时目录 after/summary.json |
| CPW-05 当前构建 ConPTY | 编译显式 ps1terminal fixture，绑定临时 capture 路径，运行 `TestCPWWindowsTerminalInput` | 回归通过后 | F8 隐藏确认不发送、切候选保留中文原草稿、窄屏两次 F2、Tab/Shift+Tab 与焦点正确，PASS | 临时目录 terminal 捕获与 VT 输出 |
| 静态及冲突 | `go vet ./internal/console`、产品构建、文档链接/任务身份/差异检查、GitHub PR 查询 | 推送前/改就绪后 | 编译/检查通过，目标头提交同步，非草稿、无冲突，必需检查无失败 | 最小审计摘要及 PR 状态 |

不重跑 Full、L2/L3 或整个 L1：本轮只修控制台显示/模式/缓存及文档冲突，既有 L1 超时与补跑证据仍保留其真实限制；没有修改服务/worker、交付或测试 runner。

## 实现状态与已有验证

## 合并前最终复审测试计划

本轮从 `cb97ce4e` 复审。只核对当前 PR 的 UI 模式、草稿和读取边界，不启动新的 roadmap/DAG 任务。

| 级别/场景 | 环境及命令 | 时点/通过条件 | 证据 |
| --- | --- | --- | --- |
| 定向 CPW-01/02/03，`TestCPWFinal*` | Windows、隔离 DTO/home；`go run ./cmd/test-feedback -out=<临时根>/before -- ./internal/console -run '^TestCPWFinal' -count=1` | 修复前真实复现退出核验、无实际候选变化、保存失败及组合文字分派问题 | 临时 before 原始报告 |
| 定向 console/query/client | `go run ./cmd/test-feedback -out=<临时根>/after -- ./internal/console ./internal/product/query ./internal/product/client -count=1` | 修复后全部通过，passed/complete=true，无 skip | 临时 after 报告 |
| 当前固定 L1 | `go run ./cmd/test-feedback -level=L1 -out=<临时根>/l1`，使用主分支当前固定范围/并行入口 | 定向通过后；passed/complete=true，保留原超时历史，不声称两版范围相同 | 临时 l1 报告 |
| 静态/合并 | go vet、构建、文档链接/任务身份/差异检查、PR 状态与精确头 SHA 合并 | 推送前/合并后；无错误、无冲突，实际 MERGED | 最小验收摘要及 GitHub 结果 |

本次不选择 Full 或 L2/L3/live；输入/提交实现没有换协议，已有 ConPTY 保留原采样证据，新增边界以定向测试验证。原始报告继续留在临时目录，仓库只记录最小摘要。

## 主分支文档整合冲突验证

本轮合并 `origin/main` 的 `ee1ab5d6`。冲突仅在 README、会话展示说明、Spec 索引及退役的控制台约束文档；控制台/product/app 实现没有冲突或改动。保留主分支的 README 精简、控制台规则集中维护和草稿/正式准入区分，将本 PR 已实现规则吸收到 `docs/product-design/execution-console.md`，删除已退役文件并更新 Spec 引用。

测试计划为纯文档静态级别：Windows，执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --cached --check`，在解决后、提交前核验链接存在、编号一致、无冲突标记及格式错误；证据见[验收摘要](../acceptance/console-primary-workspace.md)。推送后读取 PR 当前头的 draft/mergeability/check 状态，要求非草稿且无冲突。主分支的 L1 runner/样本修改沿用其已合入验证，不重复 L1/Full/L2/L3 或原生输入；本次不声称静态编号检查验证了外部 DAG。

实现、124 项发布定向回归、原地反馈接线与当前构建 Windows ConPTY 已完成；固定 L1 原集成组累计超时，原报告不完整；缺失的 TM04/TM06/TM08 补跑已全部通过，登记场景覆盖核对无遗漏。交付 [PR #234](https://github.com/erranliu/SteerLine/pull/234)，保留 L1 限制供评审。本轮审计新增有界 F8 翻页和 final/gap 缓存断言，最终 134 项回归及 ConPTY 通过。具体限制及证据见[验证记录](../acceptance/console-primary-workspace.md)，不以本计划替代验收证据。

追加最小诊断：Windows 受控环境，`go run ./cmd/test-feedback -out=docs/validation/console-primary-workspace/l1-integration-tail -- ./tests/integration -run '^Test(TM04ExecutionWiring|TM06ResourceWiring|TM08AppHostWiring)$' -count=1`；在 L1 超时后执行，要求这些原场景全部通过、passed/complete=true，证据保存在该新目录。原 L1 的 passed=false 保持，不将多个报告拼成一次成功的 L1。

最终复审：四项复现已修复，138 项定向回归与当前固定 L1 passed/complete=true；原始范围/日志与静态结果见验收摘要。按用户明确指令，在检查当前头 SHA 与平台状态后合并 PR #234。

## 自动验收覆盖与结果登记

任务专属记录：[验收文档](../acceptance/console-primary-workspace.md)。原测试计划已有环境/命令/时点/通过条件与证据，以下补齐验收项映射；原结果仅绑定原源码，不因改 Plan 重跑或重标通过。

| 自动验收项 | 对应现有入口 / 覆盖要求 |
| --- | --- |
| CPW-01–04 | 现有 console/query/client/受控 app 接线、具体 CPW 用例和 L1，各项布局/目标/草稿/回执/正文断言明确对应 |
| CPW-05 | 现有 ps1terminal TestCPWWindowsTerminalInput 与 ConPTY 的实际按键/中文/宽窄布局；不证明物理 IME |

执行安排沿本 Plan 的原测试表：实际变更触及对应验收项时运行其最小入口；未变范围先核对原证据，不默认重跑 Full/真实模型/PR 链。发现必要断言或能力缺口时先补最小场景再验收，零匹配/skip/编译不算通过。新原始报告写 runner 默认临时目录或新的空临时目录，具体命令/源码/摘要/日期、逐项结果与证据定位登记到上述任务专属记录并同步 execution-map，Plan 不作实时状态表。

原文中的历史命令/目录保持取证含义；新执行不再写 docs/validation。自动范围无需额外人工验收门槛；原生场景需人准备环境时预约窗口，不把自动失败/缺权限归类为后置人工。不新增验证矩阵或改原通过条件，本次 Plan 修订只做文档静态检查。
