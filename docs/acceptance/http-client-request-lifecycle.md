# 验收：HTTP 客户端原请求与停服观察解耦

唯一归属：任务 #93，HTTP 客户端原请求与停服观察解耦。2026-10-14经用户确认纳入[execution-map](../roadmap/execution-map.md)；尚未实施，自动项待验收。

[Spec](../spec/http-client-request-lifecycle.spec.md) · [Plan / 测试计划](../spec/http-client-request-lifecycle.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划

| ID | 行为与通过条件 | 方法、环境与时点 | 状态 |
| --- | --- | --- | --- |
| CL93-A1 | 独立调用journal/observer，核验业务HTTP不直接处理文件/OS；actual consoleapp/HTTP消费者生产闭包无业务owner，旧重复逻辑移除 | 实施新增TestHTTPClientCapabilityBoundary，原TestTUIHTTPBoundary；Windows依赖/结构检查，提取后/PR前 | 仅规划，待验收 |
| CL93-A2 | 保存失败不得POST，多窗口首次登记及后续更新均仲裁；旧submitted等观察不得覆盖applied/rejected，同终态采样/POST与GET摘要变化返回原完整回执、相反终态/原身份冲突保留原记录；原完整信封补交、unknown/旧实例禁重放、StopIdentity不可替换、Close取消 | 实施新增TestHTTPRequestJournal及原TestTUIHTTPContract；临时私有journal+httptest/stub，独立实例/Windows子进程用同步屏障覆盖两种写入顺序、两种终态、锁取消和持锁进程退出；接线后/PR前，命令见Plan | 仅规划，待验收 |
| CL93-A3 | 停服核对原applied+精确generation退出+原两锁；丢证据保持未核实，重开仍用原进程，实际TUI开始/反馈/final/退出/停服保持 | Windows原进程定向入口及test-tui-http.ps1；脚本先生成新SteerlineTest包并传root/evidence，driver显式选simulate，不使用C:\SteerlineRuntime；PR前/最终客户端接线候选 | 仅规划，待验收 |

新增测试由Plan步骤补齐后才执行。命令/级别、场景、准备与通过条件全部映射到Plan；runner原始summary/Go日志在新系统临时目录，simulate evidence在驱动新包。登记绝对目录及真实构建，不复制数据库/认证到仓库，不把simulate声明为native通过。

CL93-A2的跨进程后续更新/终态保护是本任务新增的小范围修复，基线仅有首次登记原子性；不得把移动旧saveOutgoing后既有测试通过登记为该项已验。须记录确定性交错、实际独立子进程及锁释放证据。

## 人工与执行记录

无新增人工项；#91桌面体验保持原归属和待验。自动项失败阻止本项交付，未知发送只查原请求，不能为验收重发或更换实例。

尚未执行。执行后按ID登记Spec/Plan版本、源码/dirty/构建、Windows现场、精确命令/测试数量、通过或失败、清理与绝对证据；固定L1另记回归。正式纳入后同步execution-map汇总，不回写Plan实时结果。本页只归属#93。
