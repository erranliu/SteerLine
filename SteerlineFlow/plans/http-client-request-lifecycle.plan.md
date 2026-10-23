# Plan：HTTP 客户端原请求与停服观察解耦

> 2026-10-15：TUI 专属客户端/交互已退役，本文保留原范围与历史事实；共享 HTTP、服务和进程身份要求继续有效。当前范围见[TUI 退役](../spec/tui-retirement.spec.md)。


任务 ID：93；任务名称：HTTP 客户端原请求与停服观察解耦。

依据 [Spec](../spec/http-client-request-lifecycle.spec.md)，实时结果归[专属验收](../testing/acceptance/http-client-request-lifecycle.md)。范围于2026-10-14经用户确认纳入[execution-map](../Roadmap/execution-map.md)；尚未启动实施。调查基线与RoadmapGuide/SpecGuide版本 `5a24abec3c10d184a6e57962e154854bc6011a9f`，执行时另固定实际文档版本。

## 实施步骤

1. 调查 Console 的readOutgoing/saveOutgoing/submit/lookup/StopService、Close取消、lifecycle_windows和真实构造消费者。保留Client/Open/连接握手和console消费接口，列明原信封、响应与StopIdentity不可变边界。
2. 将 outgoing值、路径/私有文件验证、原子发布/观察更新、身份/格式/限额收进同包 requestJournal。Console 用它保存/读取原记录，自己的submit/lookup仍负责原HTTP对账；删除Console内重复文件实现。不增加通用连接池或第三份响应缓存。补齐基线尚未具备的跨进程更新仲裁：在项目journal目录用每个原request ID的稳定私有锁文件，Windows用内核文件锁，既有支持的Unix目标用对应OS锁；同进程独立journal实例也必须互斥，不能只加Console mutex。不调用app/host的锁实现，不改httpfs格式/ACL helper，锁文件沿用私有文件检查/创建能力且不随JSON替换或解锁删除。以可取消等待包住重读、比较和写入；失败不写，HTTP/observer在锁外，崩溃后OS锁可释放。对applied/rejected实施first-wins终态保护：每次先沿原validateResponse核验信封/身份/目标/版本，已有终态后非终态及同终态观察返回原完整回执且不改文件，相反终态报冲突，原身份冲突始终拒绝。不要把整段CommandResponse深比较作为同终态重用条件；沿现有POST/GET检查可选字段及Code/Diagnostic摘要差异，SampledAt变化也不替换原记录。Console必须使用journal返回的实际保留结果，不能在文件保持终态时向UI返回被忽略的旧submitted。StopIdentity整体不可改。
3. 将 observeExit 作为明确的小OS观察依赖，由consoleapp构造传入默认平台实现，测试传stub。observer只取得原认证Identity，不读取journal或宿主回执。改动所有实际构造消费者，保留取消顺序与同步范围，不留旧签名兼容包装。
4. 新增 `TestHTTPRequestJournal` 和 `TestHTTPClientCapabilityBoundary`（计划入口，尚不存在）。用临时私有journal与httptest端点覆盖保存失败零POST、首次同ID竞争、unknown/旧实例/原完整信封补交、终态/停服身份不可替换；observer stub核对所见原generation/两锁。journal测试用同步屏障和独立实例/子进程确定性交错：两个窗口先取得旧submitted观察，A写applied后B尝试写submitted；反向顺序和rejected亦覆盖；最终原终态不退回。验证相同终态采样时间/POST与GET摘要或可选字段变化时仍返回原完整回执；相反终态及原身份冲突拒绝、锁等待取消、持锁子进程退出后另一进程可获锁，检查无锁/文件句柄残留。同步hook只在私有fixture，不改公共driver；不依赖sleep碰竞态。沿用现有Close/重开停服与原动作断言，不重建HTTP协议。
5. 复用 [scripts/test-tui-http.ps1](https://github.com/erranliu/SteerLine/blob/47fafc12721ceb8fa39ba1ab227872fd14513bcb/scripts/test-tui-http.ps1)：它先调用new-debug-release生成新的 C:\SteerlineDebug SteerlineTest 包，设置TestTUIHTTPRuntime所需的root/evidence环境，再由driver显式prepare/run --simulate。脚本校验summary的passed/complete/cleaned；直接运行该Go测试而未设置root会skip，不作为验收命令。无需新增业务场景/runner登记或复制安装数据。
6. 按测试表登记每项证据、源码/dirty及真实数量，更新本项客户端架构说明。实施交付后同步execution-map真实结果，本次纳入时自动验收仍待执行。

## 并行与共享段

无新增硬前置，可与#92并行；#94/#95/#96因Worker三层重构的职责/接线重叠继续留草稿，不作为本项开工前置。#92负责HTTPendpoint，本项负责consoleapp/client构造。httpcontract/API、httpfs持久格式/ACL及公共driver冻结，新增stub放本包。如果组合场景因兄弟构造变化需适配，仅后合入者更新该场景必要接线并保留行为断言。单个测试窗口有自己的临时journal，原生测试资源不默认并行。

## 测试计划

所有命令从仓库根执行，新增名称先由步骤4补齐并实际枚举。runner默认产生系统临时 `steerline-test-feedback/<运行时间>/` 的summary.json及原始Go日志/stderr；按下列行标识登记到专属验收“执行记录”。simulate脚本显式OutDir使用全新临时路径，保存runtime.log/summary.json并登记其绝对路径及新包根。

| 验收项 | 级别/组/场景、环境 | 命令 | 时点/通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [CL93-A1](../testing/acceptance/http-client-request-lifecycle.md#自动验收计划) | 定向owner / 新增 TestHTTPClientCapabilityBoundary、既有应用消费者闭包；Windows普通Go | `go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^TestHTTPClientCapabilityBoundary$' -count=1`；`go run ./cmd/test-feedback -- ./internal/app -run '^TestTUIHTTPBoundary$' -count=1` | 抽取后、PR前；窄能力独立，直接/传递闭包无后端，零匹配/skip失败 | 两个runner目录；CL93-A1 / boundary |
| [CL93-A2](../testing/acceptance/http-client-request-lifecycle.md#自动验收计划) | 定向owner / 新增TestHTTPRequestJournal及TestTUIHTTPContract；私有文件+httptest、Windows独立实例/受控子进程与同步屏障 | `go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^(TestHTTPRequestJournal|TestTUIHTTPContract)$' -count=1` | 接线后、PR前；首次登记/后续跨进程写入均仲裁，applied/rejected不被旧观察覆盖，同终态摘要变化保留原回执、相反终态/原身份冲突拒绝，锁取消/进程退出可收敛；保存失败零POST、Close取消通过 | runner目录；CL93-A2 / requests，登记两种交错/子进程退出及实际子场景数 |
| [CL93-A3](../testing/acceptance/http-client-request-lifecycle.md#自动验收计划) | Windows停服观察及独立simulate组合 / 既有TestTUIHTTPRuntime | `go run ./cmd/test-feedback -- ./internal/product/httpclient -run '^TestTUIHTTPStopReopenUsesOriginalProcess$' -count=1`；`.\scripts\test-tui-http.ps1 -OutDir (Join-Path $env:TEMP ('steerline-cl93-' + [guid]::NewGuid().ToString('N')))` | PR前及最终共享客户端构造候选；原三证据/丢回执/重开身份、窗口关闭后服务继续、实际操作及清理通过 | 停服runner目录及脚本OutDir的runtime.log/summary.json；CL93-A3 / stop、simulate |
| 必要回归 | 固定L1，隔离fixture | `go run ./cmd/test-feedback -level=L1` | PR前一次，passed/complete且闭包通过 | L1目录；回归记录 |

不跑Full、付费L2/L3或原生PR：不改变业务流程/后端协议，独立simulate与精确Windows OS样本覆盖本项；不以simulate作为原生模型/桌面体验证明。无新增人工窗口；#91人工体验另保留。
