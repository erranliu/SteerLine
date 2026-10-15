# 验收：HTTP 客户端原请求与停服观察解耦

> 2026-10-15：TUI 专属客户端/交互已退役，本文保留原范围与历史事实；共享 HTTP、服务和进程身份要求继续有效。当前范围见[TUI 退役](../spec/tui-retirement.spec.md)。


唯一归属：任务 #93，HTTP 客户端原请求与停服观察解耦。2026-10-14经用户确认纳入[execution-map](../roadmap/execution-map.md)；实现、独立复审修复、必要自动验证和[PR #312](https://github.com/erranliu/SteerLine/pull/312)真实合并均已核验；CL93-A1–A3通过，无新增人工项。旧CL93-A2覆盖声明已校准，历史证据保留如下。

[Spec](../spec/http-client-request-lifecycle.spec.md) · [Plan / 测试计划](../spec/http-client-request-lifecycle.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划

| ID | 行为与通过条件 | 方法、环境与时点 | 状态 |
| --- | --- | --- | --- |
| CL93-A1 | 独立调用journal/observer，核验业务HTTP不直接处理文件/OS；actual consoleapp/HTTP消费者生产闭包无业务owner，旧重复逻辑移除 | 实施新增TestHTTPClientCapabilityBoundary，原TestTUIHTTPBoundary；Windows依赖/结构检查，提取后/PR前 | 通过；2条定向命令各1测试、0失败/跳过，证据见执行记录 |
| CL93-A2 | 保存失败不得POST，多窗口首次登记及后续更新均仲裁；旧submitted等观察不得覆盖applied/rejected，同终态采样/POST与GET摘要变化返回原完整回执、相反终态/原身份冲突保留原记录；原完整信封补交、unknown/旧实例禁重放、StopIdentity不可替换、Close取消 | 实施新增TestHTTPRequestJournal及原TestTUIHTTPContract；临时私有journal+httptest/stub，独立实例/Windows子进程用同步屏障覆盖两种写入顺序、两种终态、锁取消和持锁进程退出；接线后/PR前，命令见Plan | 通过（修正后）；30项测试/子测试事件、0失败/跳过；四种交错由8个独立子进程实际执行且门禁前观察均为非空submitted |
| CL93-A3 | 停服核对原applied+精确generation退出+原两锁；丢证据保持未核实，重开仍用原进程，实际TUI开始/反馈/final/退出/停服保持 | Windows原进程定向入口及test-tui-http.ps1；脚本先生成新SteerlineTest包并传root/evidence，driver显式选simulate，不使用C:\SteerlineRuntime；PR前/最终客户端接线候选 | 通过；定向Windows停服用例与隔离simulate场景通过并清理；simulate记录1个conversation gap、0个final body，不据此声称native final通过 |

新增测试由Plan步骤补齐后才执行。命令/级别、场景、准备与通过条件全部映射到Plan；runner原始summary/Go日志在新系统临时目录，simulate evidence在驱动新包。登记绝对目录及真实构建，不复制数据库/认证到仓库，不把simulate声明为native通过。

CL93-A2的跨进程后续更新/终态保护是本任务新增的小范围修复，基线仅有首次登记原子性；不得把移动旧saveOutgoing后既有测试通过登记为该项已验。须记录确定性交错、实际独立子进程及锁释放证据。

## 人工与执行记录

无新增人工项；#91桌面体验保持原归属和待验。自动项失败阻止本项交付，未知发送只查原请求，不能为验收重发或更换实例。

### 执行记录

固定文档版本：Spec、Plan及本验收文档均取自 35d4d65350fa6f6f284e9e91a6bd657ba7abb947；实现基线为最新主干 ab814da9d5fcec17f3ba46b59827734d171ba6b7。测试环境为 windows/amd64、Go 1.26.4、主机 liuming-asus。正向runner均 passed=true、complete=true且零失败/跳过；mutation反证按预期以两个目标子测试失败退出1。完整Go日志和summary位于各自绝对目录。

| 记录 | 命令 / 结果 | 证据 |
| --- | --- | --- |
| CL93-A1 / capability | go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^TestHTTPClientCapabilityBoundary$' -count=1；1/1通过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094502.271960800Z\summary.json |
| CL93-A1 / app closure | go run ./cmd/test-feedback -- ./internal/app -run '^TestTUIHTTPBoundary$' -count=1；1/1通过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094520.015118000Z\summary.json |
| CL93-A2 / 旧运行范围校准 | 旧命令结果为30/30，但当时四种interleaving的stale helpers均只有 Envelope、Response=nil；只证明空观察场景不替换终态，不能作为submitted覆盖保护证据。reviewer的nonterminal mutation当时仍以19/19通过，证明旧断言没有覆盖此回归 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094551.660381300Z\summary.json；协调review报告 C:\Users\lmas1\.codex\task-state\roadmap-runner\roadmap-steerline-http-20261014-01a13923\review93-round1.txt；旧mutation C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-review-nonterminal-mutation-2261dbdc\overlay_overwrite.json、go-test-overwrite.jsonl |
| CL93-A2 / requests（修正后） | go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^(TestHTTPRequestJournal|TestTUIHTTPContract)$' -count=1；30/30测试及子测试通过，0失败/跳过。applied/rejected各覆盖terminal-first与stale-submitted-first；四个场景共启动8个独立子进程，双方在门禁前写出同ID、同完整信封、同非空submitted响应的观察快照。terminal-first的stale写尝试返回完整保留终态；stale-first先真实写submitted，再由终态更新覆盖；最终完整响应与磁盘记录一致 | runner summary C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T104223.419884700Z\summary.json；集中证据目录 C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-a2-fix-d8797a0dfbe3464e8eb0132813a8b29a |
| CL93-A2 / mutation反证 | 仅对TestHTTPRequestJournal运行临时requestJournal nonterminal-overwrite overlay：go test -overlay=C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-a2-fix-d8797a0dfbe3464e8eb0132813a8b29a\overlay_overwrite.json -json -count=1 -run '^TestHTTPRequestJournal$' ./internal/product/httpclient；退出码1，恰好两个目标场景失败：applied-before-stale-submitted、rejected-before-stale-submitted。确认加强后的断言能确定性检出submitted覆盖终态；仓库生产实现未被mutation修改 | C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-a2-fix-d8797a0dfbe3464e8eb0132813a8b29a\evidence-summary.json、go-test-overwrite.jsonl、overlay_overwrite.json、request_journal_overwrite.go |
| CL93-A3 / 原进程观察 | go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^TestTUIHTTPStopReopenUsesOriginalProcess$' -count=1；1/1通过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094622.977446000Z\summary.json |
| CL93-A3 / simulate组合 | scripts/test-tui-http.ps1 -OutDir C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-e5c914cd5fbb4f4eba272deebc193daf；passed/complete/cleaned=true，workflow turns=2。停服原applied receipt、原进程退出及两锁释放均为true；canonical final查询记录1 gap、0 body | C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-e5c914cd5fbb4f4eba272deebc193daf\summary.json 与同目录 runtime.log；隔离包 C:\SteerlineDebug\dev-20261014-174645-ab814da9-dirty；二进制SHA-256 faec8ffb32e5508fca987db2bc685747a3e51bf67fbacd89a40b4d1d82080915 |
| CL93-A3 / clean production候选复用 | 独立reviewer在PR候选提交2261dbdc上运行；此轮仅修改测试/验收文件，生产代码未变。source_dirty=false，simulate passed/complete/cleaned=true，workflow turns=3，原applied receipt、进程退出及两锁释放均true；仍为1 gap、0 body，不声明native final通过 | C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-review-final-2261dbdc-20261014\summary.json、runtime.log；隔离包 C:\SteerlineDebug\dev-20261014-182218-2261dbdc |
| 固定L1回归 | go run ./cmd/test-feedback -level=L1；quick scope完整通过，26包、553测试通过，0失败、0跳过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T095215.852912700Z\summary.json |

先前的CL93-A2 30/30仅覆盖空观察，现已由修正后的30/30真实submitted观察、8个子进程交错和mutation反证取代；不得引用旧结果宣称四种submitted交错已验。simulate包只在 C:\SteerlineDebug 下生成；没有使用 C:\SteerlineRuntime。没有新增人工项；#91桌面体验保持原归属和待验。自动项失败阻止本项交付，未知发送只查原请求，不能为验收重发或更换实例。实现提交为 7c7bd5c2465d3c503fce623bdbfa8afd776369e0；PR [#312](https://github.com/erranliu/SteerLine/pull/312)已通过新的独立终审并真实合并；最终证据见下节。本页只归属#93。

## 最终独立复审与真实合并

2026-10-14，首轮 reviewer `/root/review93_round1` 确认旧跨进程场景的 stale 响应为 nil，未覆盖真实 submitted 覆写终态；由原执行聊天 `01a139bc-1014-7051-a871-ebb839be753b` 在 `641fc79628327071451ae95104a86600eaa017da` 补强私有测试和校准验收。最终新 reviewer `/root/review93_round2` 检查完整17文件diff、固定Spec/Plan、生产请求/停服路径、全部构造消费者和原始证据，未发现新增阻塞；唯一P2已闭合。

最终reviewer在clean head `641fc79628327071451ae95104a86600eaa017da`执行固定CL93-A2命令，30项通过、passed/complete=true、exit0、无失败/skip；summary：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261014T110042.016876000Z/summary.json`，同目录原始JSON/stderr已核。四组合共8子进程在门禁前实际读取并断言非空完整submitted；两种次序、applied/rejected、磁盘和retained完整终态均验证。独立重跑临时nonterminal-overwrite变体，仅两个terminal-first叶子在terminal was lost断言失败、exit1、无skip；日志：`C:/Users/lmas1/AppData/Local/Temp/steerline-cl93-review2-mutation-14d14d20767048fab67b545682c9821e/go-test-overwrite.jsonl`。旧nil结果不再作为submitted保护已验依据。

生产代码及producer相对clean `2261dbdc`组合候选未变，A1两入口/A3原进程、clean定向32项、固定L1 12组/26包/553项及独立simulate按原始事件核验后复用。simulate的原journal PID56424、generation01dd5bc60d1340db及原包两锁绑定，原applied、精确退出和两锁释放齐备，passed/complete/cleaned=true；1 gap、0 body、native_final_verified=false是冻结驱动原支持边界，不表示native final通过。

[PR复审记录](https://github.com/erranliu/SteerLine/pull/312#issuecomment-6167152543)保留agent身份、发现/修复提交、实际验证和限制。合并前重新核head/base未变，base `ab814da9d5fcec17f3ba46b59827734d171ba6b7`、MERGEABLE/CLEAN、REST main未保护、GraphQL保护规则/rulesets均0、无必需检查；按预期head合并。GitHub确认 `mergedAt=2026-10-14T11:20:42Z`（北京时间19:20:42），merge `a1e38696f44a48f6db4946f62b137de38daded91`已进入origin/main，父提交与上述base/head一致。实现完成及CL93-A1–A3自动验收通过分别登记，无本任务人工残留；#91原桌面体验不因本次交付标为通过。
