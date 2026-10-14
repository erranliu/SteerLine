# 验收：HTTP 客户端原请求与停服观察解耦

唯一归属：任务 #93，HTTP 客户端原请求与停服观察解耦。2026-10-14经用户确认纳入[execution-map](../roadmap/execution-map.md)；实现及自动验收已完成，自查后待独立只读复审与真实PR合并。

[Spec](../spec/http-client-request-lifecycle.spec.md) · [Plan / 测试计划](../spec/http-client-request-lifecycle.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划

| ID | 行为与通过条件 | 方法、环境与时点 | 状态 |
| --- | --- | --- | --- |
| CL93-A1 | 独立调用journal/observer，核验业务HTTP不直接处理文件/OS；actual consoleapp/HTTP消费者生产闭包无业务owner，旧重复逻辑移除 | 实施新增TestHTTPClientCapabilityBoundary，原TestTUIHTTPBoundary；Windows依赖/结构检查，提取后/PR前 | 通过；2条定向命令各1测试、0失败/跳过，证据见执行记录 |
| CL93-A2 | 保存失败不得POST，多窗口首次登记及后续更新均仲裁；旧submitted等观察不得覆盖applied/rejected，同终态采样/POST与GET摘要变化返回原完整回执、相反终态/原身份冲突保留原记录；原完整信封补交、unknown/旧实例禁重放、StopIdentity不可替换、Close取消 | 实施新增TestHTTPRequestJournal及原TestTUIHTTPContract；临时私有journal+httptest/stub，独立实例/Windows子进程用同步屏障覆盖两种写入顺序、两种终态、锁取消和持锁进程退出；接线后/PR前，命令见Plan | 通过；30个测试/子测试事件、0失败/跳过；四种applied/rejected交错、首次登记竞争及持锁子进程退出均有证据 |
| CL93-A3 | 停服核对原applied+精确generation退出+原两锁；丢证据保持未核实，重开仍用原进程，实际TUI开始/反馈/final/退出/停服保持 | Windows原进程定向入口及test-tui-http.ps1；脚本先生成新SteerlineTest包并传root/evidence，driver显式选simulate，不使用C:\SteerlineRuntime；PR前/最终客户端接线候选 | 通过；定向Windows停服用例与隔离simulate场景通过并清理；simulate记录1个conversation gap、0个final body，不据此声称native final通过 |

新增测试由Plan步骤补齐后才执行。命令/级别、场景、准备与通过条件全部映射到Plan；runner原始summary/Go日志在新系统临时目录，simulate evidence在驱动新包。登记绝对目录及真实构建，不复制数据库/认证到仓库，不把simulate声明为native通过。

CL93-A2的跨进程后续更新/终态保护是本任务新增的小范围修复，基线仅有首次登记原子性；不得把移动旧saveOutgoing后既有测试通过登记为该项已验。须记录确定性交错、实际独立子进程及锁释放证据。

## 人工与执行记录

无新增人工项；#91桌面体验保持原归属和待验。自动项失败阻止本项交付，未知发送只查原请求，不能为验收重发或更换实例。

### 执行记录

固定文档版本：Spec、Plan及本验收文档均取自 35d4d65350fa6f6f284e9e91a6bd657ba7abb947；实际测试基线为最新主干 ab814da9d5fcec17f3ba46b59827734d171ba6b7，运行时源码含本分支未提交候选改动（runner记为dirty）。环境为 windows/amd64、Go 1.26.4、主机 liuming-asus。下面各runner均 passed=true、complete=true，定向项零失败/跳过；完整Go日志和summary位于各自绝对目录。

| 记录 | 命令 / 结果 | 证据 |
| --- | --- | --- |
| CL93-A1 / capability | go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^TestHTTPClientCapabilityBoundary$' -count=1；1/1通过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094502.271960800Z\summary.json |
| CL93-A1 / app closure | go run ./cmd/test-feedback -- ./internal/app -run '^TestTUIHTTPBoundary$' -count=1；1/1通过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094520.015118000Z\summary.json |
| CL93-A2 / requests | go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^(TestHTTPRequestJournal|TestTUIHTTPContract)$' -count=1；30/30测试及子测试通过。四个旧submitted/applied/rejected次序、独立实例竞争、首次双进程登记、相同终态完整回执、冲突保留、取消及持锁子进程退出均实际执行 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094551.660381300Z\summary.json |
| CL93-A3 / 原进程观察 | go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^TestTUIHTTPStopReopenUsesOriginalProcess$' -count=1；1/1通过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T094622.977446000Z\summary.json |
| CL93-A3 / simulate组合 | scripts/test-tui-http.ps1 -OutDir C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-e5c914cd5fbb4f4eba272deebc193daf；passed/complete/cleaned=true，workflow turns=2。停服原applied receipt、原进程退出及两锁释放均为true；canonical final查询记录1 gap、0 body | C:\Users\lmas1\AppData\Local\Temp\steerline-cl93-e5c914cd5fbb4f4eba272deebc193daf\summary.json 与同目录 runtime.log；隔离包 C:\SteerlineDebug\dev-20261014-174645-ab814da9-dirty；二进制SHA-256 faec8ffb32e5508fca987db2bc685747a3e51bf67fbacd89a40b4d1d82080915 |
| 固定L1回归 | go run ./cmd/test-feedback -level=L1；quick scope完整通过，26包、553测试通过，0失败、0跳过 | C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T095215.852912700Z\summary.json |

simulate包只在 C:\SteerlineDebug 下生成；没有使用 C:\SteerlineRuntime。没有新增人工项；#91桌面体验保持原归属和待验。自动项失败阻止本项交付，未知发送只查原请求，不能为验收重发或更换实例。实现提交为 7c7bd5c2465d3c503fce623bdbfa8afd776369e0；PR [#312](https://github.com/erranliu/SteerLine/pull/312)已创建，待独立只读复审，本轮不合并。本页只归属#93。
