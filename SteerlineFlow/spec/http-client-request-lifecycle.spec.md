# Spec：HTTP 客户端原请求与停服观察解耦

> 2026-10-15：TUI 专属客户端/交互已退役，本文保留原范围与历史事实；共享 HTTP、服务和进程身份要求继续有效。当前范围见[TUI 退役](tui-retirement.spec.md)。


任务 ID：93；任务名称：HTTP 客户端原请求与停服观察解耦。

范围于2026-10-14经用户确认纳入[execution-map](../Roadmap/execution-map.md)，尚未实施；实时执行/验收状态归正式清单和专属验收。配套 [Plan](../plans/http-client-request-lifecycle.plan.md)、[专属验收](../testing/acceptance/http-client-request-lifecycle.md)、[本轮草稿](../Roadmap/drafts/roadmap.md)。调查基线 `5a24abec3c10d184a6e57962e154854bc6011a9f`；实际启动固定文档版本与最新主干。

## 问题与成果

[httpclient/console.go](https://github.com/erranliu/SteerLine/blob/47fafc12721ceb8fa39ba1ab227872fd14513bcb/internal/product/httpclient/console.go) 同时处理页面消费、原信封文件、提交/查找对账和 Windows 原进程/两锁退出判断。[client.go](../../internal/product/httpclient/client.go) 已有原始 HTTP 请求和连接能力，无需再建设连接管理框架。

在现有 httpclient 包中抽出命名的原请求 journal owner，统一 outgoing 的路径、格式/原身份核验、首次原子登记、观察更新和原响应绑定。Console 的提交/lookup 继续组织原 HTTP 对账，但不再直接实现文件持久化。把停服 OS 观察作为显式注入的小端口/函数，consoleapp 负责绑定当前平台默认实现；业务 HTTP 查询、提交与 journal 不调用 OS 观察。现有 Console 消费接口、实际取消和重连行为保持，删除旧重复文件逻辑。

本项另包含一个明确的小范围修复：当前 saveOutgoing 的“读取旧记录—检查—替换”仅受单个 Console 的 mutex 保护，首次 Link 原子登记不能防止跨窗口后续更新把 applied/rejected 覆盖成旧的 submitted。journal 负责将同项目/原 request ID 的全部写入跨进程串行化，并在写入临界区重读后判断；这项终态保护是新增交付要求，不声称基线已有。收益是原请求持久化有单一 owner，且两个窗口交错观察时仍保留已取得的最终回执。

## 契约与约束

- journal 只保存客户端原请求、已取得回执与经认证的原停服身份，不成为业务权威。完整原信封和 StopIdentity 首次发布后不可修改，观察更新不能覆盖原目标、版本、payload 或终态回执。
- 原信封先保存后发送；保存失败零 POST。多窗口竞争的同 ID 原子登记、同内容重用及异内容冲突保持，现有私有文件/DACL、编码和大小限制保持。
- 跨窗口写入使用独立于可替换 JSON 的稳定私有锁文件；锁覆盖重读、内容核验与原子发布/替换，不覆盖 HTTP 或退出观察。锁等待响应取消，进程退出由 OS 释放锁，锁文件不在释放后删除，避免不同文件对象各持一把锁。JSON 格式不变；辅助锁归本项 journal，不复用宿主业务锁。获锁/核验失败不写记录，首次登记失败不 POST。
- applied/rejected 是不可回退的终态：先核验每次观察与原信封/目标/版本的绑定；已有终态后，非终态和同终态观察均返回原已保存的完整回执，不替换原内容，相反终态须报冲突并保留原记录。同终态不同摘要形态不是冲突：现有POST与GET可能有不同Code/Diagnostic或可选嵌套字段，SampledAt也会变化；不能直接比较整个CommandResponse拒绝合法重复查询。身份/目标/版本冲突始终拒绝。StopIdentity连同首次保存的采样身份仍不可修改。
- 未知发送先查原 ID；只有正式 command_not_found 才按原完整信封补交。旧实例、读取失败、权限错误、outcome_unknown 均不生成新 ID、不重定向目标、不读宿主业务文件。
- 停服 observer 仅依据保存的原认证 Identity 核验精确 process generation 退出和原两锁释放，不获得 HTTP命令、journal 写入、Task 或 worker 能力。只有原 applied 回执与退出/两锁事实齐备才能显示 stopped，缺证据保留未核实。
- 保留 Close 先取消网络工作再等待同步的顺序，HTTP生命周期、超时、窗口/项目缓存及 UI 草稿语义保持。非 Windows 的实际观察限制保持，不新增远程模式、业务库回退、SSE 或连接重试策略。

## 所有权与并行

本项拥有 httpclient 内 journal/生命周期观察及所属测试、consoleapp 必要构造接线。保持现有 Client/Open 和 console 消费接口；必要构造参数变化只由本项一次更新全部实际消费者，不留旧构造兼容转发。#92 不修改本包，本项不修改 httpapi、公共 wire/API、httpfs 格式和 ACL 实现。

无新增任务硬前置，复用 #91；可与 #92 并行；正在实施的Worker三层重构保持公开HTTP/DTO，本项不改其内部执行接线。#94/#95/#96继续留草稿，不构成本项前置。测试中可用自己的 httptest/stub 端点；它不代表真实服务集成已验。实际 TUI/HTTP 组合回归用原应用隔离夹具。本项不重写 TUI Model，不建立未来客户端框架。

## 最小验收

| ID | 自动通过条件 |
| --- | --- |
| CL93-A1 | journal 和 OS observer 有单一职责及独立测试入口；业务调用不直接访问私有文件或 OS观察，客户端依赖闭包继续隔离后端 |
| CL93-A2 | 先保存后发送、保存失败、多窗口首次登记和后续写入仲裁、冲突/旧实例/unknown/补交与重开查询保持；旧非终态不得覆盖 applied/rejected，相反终态/原身份冲突保留原记录，同终态摘要变化仍保留原完整回执，原信封及停服身份不可替换 |
| CL93-A3 | 原 applied/精确退出/两锁三证据、丢回执、旧进程身份重开、Close取消及 TUI原操作通过隔离组合验证 |

无新增人工项，真实桌面体验继续归 #91。具体自动步骤与状态归专属验收。
