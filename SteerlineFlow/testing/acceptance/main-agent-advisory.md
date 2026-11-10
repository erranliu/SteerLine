# 主 Agent：generate 接入验收

状态：按 2026-11-09 用户确认，复用现有 Runner generate/1，本地 Codex 只读生成；来源为开发候选明确待答问题（包括普通待答）。核心独立且唯一业务产物仍为后续对话提示词，采用与执行在外部。合并状态见[PR #478](https://github.com/erranliu/SteerLine/pull/478)，未部署。正式目标见 [Spec](../../spec/main-agent-advisory.spec.md)、[Plan](../../plans/main-agent-advisory.plan.md)及[测试选择](../../plans/main-agent-advisory.tests.json)。

此前严格零工具前置及生产永久阻断已被用户明确替换，[初始阻断草稿](main-agent-advisory-blocked-draft.md)及[PR475 原实现](main-agent-materials.md)仅作历史。原生工具/隐式上下文仍按 generate 既有能力，不声称严格零工具或单目录隔离；SteerLine 没有直连 Codex/app-server/模型 API。

## 当前验收映射

| ID | 实施与验证范围 | 状态 |
| --- | --- | --- |
| A01 | contract 仅 stdlib；核心只依赖 contract/prompts；显式详情 DTO不暴露执行记录 | 定向与独立静态审核通过 |
| A02 | 冻结全文、UTF-8/数量/字节约束；按绑定 ref commit/SHA 读取；准备失败保留已取得材料和诊断 | 核心/宿主定向及真实 SQLite 来源集成通过 |
| A03 | 唯一非空 prompt；旧 JSON 动作对象拒绝；结果实际来源与原文一致 | 定向与真实 generate 通过 |
| A04 | 只经 Runner generate/1，原 compose 本地 Sol/high/read_only，复用公开提交/观察/停止 | 实际真实链通过；不再要求零工具 capability |
| A05 | 完整固定文本与显式材料进入 Instruction，空 materials，无 repository/directory/file/output_schema | 实际请求断言、真实输入样本及独立审核通过；隐式原生材料不作隔离保证 |
| A06 | 无原题答案、文件发布或隐藏业务执行入口 | 旧动作拒绝定向通过，HTTP 全集通过（本轮） |
| A07 | 原请求去重、started未知只查、共享队列、取消/源解除/结算、冷态落定与原停止恢复 | bridge 与恢复定向通过，真实服务/资源停止核实；部署全集通过（本轮） |
| A08 | 默认模型与设置冻结；最小只读上下文/正文、复制及取消；未结算继续轮询 | generate阶段352/352；L4增补阶段389/389及真实Edge导航/上下文/复制/取消通过，人工主观体验未验 |
| A09 | 同包固定材料、缺失失败、实际全文 diff/字节/hash/golden/发送守卫 | 独立静态审核通过，最终证据已独立复核通过 |
| A10 | advisory-v1 固定，不接受未知协议/能力或旧动作产物 | 定向通过 |
| A11 | 原 owner 持久当前 Question/Attempt/CanReply 自动登记，不依赖诊断文案或虚构原因码 | 真实 SQLite 候选保存→后台→生成→活动→重启去重集成通过；原生问题由公开 fixture 提供，未跑完整真实开发任务 |
| A12 | 单一正式活动来源、原请求/精确导航、attention、纯读与有界正文剥离 | 集成与投影定向通过，SQLite 全集串行复核通过（本轮） |
| A13 | 不打开页面亦推进、跨候选隔离、来源解除不改历史、重启不重投 | 集成与定向、正式L4通过；使用者人工主观体验未验 |
| A14 | 稳定受控L4、真实触发至活动/浏览器、未知/重启/取消/共享槽与必要证据 | 初版04724814连续两轮；2026-11-10 cdf5bd77补普通进度取消/未知停止恢复后通过，独立复核通过 |

## 定向及真实链证据

本轮受测基础为 `448ddda0` 上的 dirty worktree，完整命令和各次实际源码/工作区状态在原报告中。Runner 源码未修改，无需 Runner worktree。SQLite schema 44，不迁移之前开发数据。

报告根 `C:/Users/lmas1/AppData/Local/Temp/`，每次使用独立 out：

| 范围 | 目录 | 结果 |
| --- | --- | --- |
| 首轮 bridge/来源/核心/提示词守卫 | `steerline-advisory-generate-owner1` | 通过，5.9 秒 |
| 补强结果绑定后的 owner | `steerline-advisory-generate-owner2` | 通过，47.4 秒 |
| 持久来源集成首次 | `steerline-advisory-real-source1` | 失败：SQL related.settled 为数字，投影 bool 解码拒绝；不是生成失败 |
| 持久来源集成修复复验 | `steerline-advisory-real-source2` | 通过，42.6 秒；此来源从 payload 读取结算，不再重复写 related 数字 |
| 冷态恢复及原停止身份 | `steerline-advisory-recovery1` | 通过，33.3 秒；未取消/已取消 absent 均落定并释放槽，failed-stop保留原unknown信封，两次准备不换ID |
| 真实 generate 首轮 | `steerline-advisory-generate-native1` | 通过，23.4 秒；原证据 `steerline-advisory-generate-native-result1.json` |
| 最终完整 Final 绑定真实复验 | `steerline-advisory-generate-native2` | 通过，27.9 秒；原证据 `steerline-advisory-generate-native-result2.json` |

定向用 `go run ./cmd/test-feedback -out=<本次目录> -- <具体包> -run <明确Test过滤> -count=1 -timeout=<预算>`；真实用例位于 bridge、带 runneracceptance 标签并只选 TestAdvisoryLocalCodexGeneratePublished。环境显式 STEERRUNNER_EXE、MAIN_AGENT_CODEX_EXE、MAIN_AGENT_LIVE_EVIDENCE。没有标签测试的包只算编译，不将零匹配/skip算通过。

真实链使用固定 `C:/Project/SteerRunner/bin/steerrunner.exe`：`0.2.0-dev.4`，SHA `d15da21b7b82e21f7347c72404d282f4432b963dcc97db5825595d09676eae6d`，资源摘要 `910c7fa06b933be3632f862062e966200fd581b34873a395268826d44c1bbbe7`。原生使用已配置本地 Codex EXE、模型 gpt-6.1-sol/high，经 Runner compose 冻结 read_only。每次创建新临时 state-dir，不使用安装环境，不创建开发候选或真实 PR。

[真实请求/来源/唯一正文及清理摘要](evidence/main-agent-generate-live.json)保存实际完整 generate.Instruction（1665 UTF-8字节）、摘要、空材料与无仓库字段、原 request/Run、InputDigest、verified Final/Source一致及 stop_status。样本仅移除执行器本机路径/运行目录，不修改模型实际输入或提示词；原记录文件另有 hash。模型产出经过核心 Accept，Service.Close、Runner StopService 和 VerifyServiceSettlement均通过后删除隔离执行现场。这个真实测试不证明完整真实开发任务产出 Question；真实来源生产字段沿既有 Runner Question/CanReply 契约，生产适配已由持久来源集成验证。

## 提示词差异及独立审核

用户放宽的是推理约束，模块唯一产物和外部执行边界不变。[本轮零上下文实际 diff](evidence/main-agent-generate-prompts.diff)以 `448ddda0` 为基线。唯一行为正文变化：将“你没有工具和项目访问能力。不得读取文件……”改为“本次仅需理解已提供的正文，不要读取其他文件……”，不再把提示词约定声称为工具硬保证；保留不编造、只输出一段提示词、不执行业务操作。

| UTF-8 资产 | 本轮前 | 本轮后 |
| --- | ---: | ---: |
| 短模板 main-agent.md | 145 B | 145 B |
| 完整规则 materials/main-agent.md | 1019 B | 1025 B |
| 代表性完整组装 golden | 1174 B | 1180 B |

规则 SHA `3a7a447bbe7527bf4c1c46867f9180cab32bc3b61359a92d9db9784172c38189`；golden SHA `8333a709f40e5fa818fd0562f5bba3bfc1ebef3e0afd13dcf0902356eb36e6d1`。1180 B 仍是 goal=clarify/空材料样本，1665 B 才是本次真实请求的完整组装正文，不能混为工具执行审计。

独立 reviewer `/root/independent_review` 已审核实际规则、字节/hash、正式 Instruction 发送点、空材料及无仓库字段、核心依赖与宿主生命周期，结论为当前 generate/1 read_only 用户授权范围静态审核通过。发现并修复：全部 started 未结算的冷态不存在请求落定、共享槽释放、失败停止意图恢复、完整 Final/command/attempt来源绑定、取消中及失败未结算的持续轮询。审核者未运行测试；当前聊天运行上述自动验证。黄金样本更新不替代独立批准。

## 完整回归与剩余边界

本轮 HTTP 全集54.0秒通过；固定提交649068ef的部署全集707.4秒、Web全集352/352（33.6秒）、生产构建通过。合并main后owner定向65.9秒通过。存储在合并前首次全集163.8秒通过；最新基线与部署并行时全集127.3秒失败，仍是原 TestAsyncActivityPaginationAndRequestLookup 的 association lookup depended on page，未确证原因、未改断言或预算，部署结束后在8c801a65串行全集复核101.2秒通过。最终L1 18.1秒、L2 106.2秒通过，L2清理完成；最终文档检查如下登记；不继承初始草稿的全集通过。最终受测提交已固定，并导出正式 Git 测试历史。人工浏览器可读性、复制及导航未验；没有合并、部署、真实开发/PR执行，亦不声明严格零工具能力。

补充状态修复：同一待答问题短暂退出又恢复时只恢复 SourceActive，不重开 ready/failed/cancelled/cancelling，不替换原 Source、请求、输入、结果或停止身份。已补 bridge 四态断言与真实 SQLite 来源恢复后的 attention 集成断言。部署/Web通过及存储失败的固定提交结果已导出[649068ef 正式历史](../records/649068efe0d5eada0b92b82e5da06a867647c7e9.json)，后续结果按实际 SHA 分别登记。

## 最终固定提交与发布前检查

来源恢复修复提交 `8c801a656ebd641083caefeded05a3afa3f6d50e` 在前后干净、源码未变的状态下通过：

| 正式检查 | 系统临时目录 | 结果 |
| --- | --- | --- |
| go.storage 串行完整复核 | `steerline-advisory-generate-storage-serial` | 通过，101.2秒；原分页断言与来源预算未改 |
| l1.minimum | `steerline-advisory-generate-L1-final` | 通过，18.1秒 |
| smoke.delivery（L2） | `steerline-advisory-generate-L2-final` | 通过，106.2秒；cleanup_complete=true |

[8c801a65 正式历史](../records/8c801a656ebd641083caefeded05a3afa3f6d50e.json)与前述649068ef历史分别保留实际受测版本；后续仅追加本验收及记录，不冒认新文档提交已受测。来源恢复定向 `steerline-advisory-source-return-final` 6项通过（39.95秒），独立复核确认只更新 SourceActive、不重开生成。

本轮 HTTP/SQLite/部署/Web完整集及生产构建均已验证，60项owner定向、真实模型与对应失败/补测分别留原报告。全量 Roadmap 未重跑：本轮没有改 Roadmap 实现、提示词或资源身份；复用其 ContentQueue 不改变其流程。人工浏览器体验仍未验，不声称完整真实开发任务或严格工具隔离。独立最终文档/证据复核已通过，提示词diff仍与当前资产/守卫逐字一致。

正式测试选择全部为现有有效ID、无重复；仓库完整PR diff空白检查通过。文档本地导航检查最终结果随本段收尾复核，不改源码/提示词。未合并、未部署。

最终导航复核：724个Markdown、4468条本地链接、0错误；完整PR diff空白检查通过。最终收尾只改Spec状态、验收说明及正式测试历史，运行代码和提示词保持受测版本。

## 稳定 L4 增补（2026-11-09）

正式 ID 为 `runtime.main-agent-advisory`（L4 / controlled），已接入 manifest、正式清单、Spec/Plan、Web构建及串行选择、必要证据校验。真实应用、Git、SQLite、HTTP、Edge运行；仅在公开Runner对端脚本化原Question、generate回执/结果和结算。没有直接写业务数据库或调用内部建议登记/生成入口。

```powershell
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario runtime.main-agent-advisory -out <全新绝对目录>
```

一次场景覆盖六个作业：自动成功、提交回执丢失、生成失败、用户取消、取消未结算时的新作业排队、来源解除。第一段建议在启动Web/浏览器之前生成；同一来源消失/恢复和服务重启不换快照、不重投。未知提交先证实对端接收且原查询失败，在未知记录仍持久时停止所属宿主，再恢复对端可查询后启动，沿原Run恢复并完成，提交始终一次；不要求查询不可用时绕过既有启动阻断。

真实浏览器验证活动中心“需处理”精确导航、冻结上下文、唯一正文和实际剪贴板，以及显式取消；面板没有采用、答案、材料PR或执行入口。取消及来源解除只停止对应建议，公开stop受理不等于结算；占用未释放时下一个作业已有冻结上下文但不能Submit。全文按原advisory ID和公开冻结Context同包组装后，与实际Instruction逐字匹配；Sol/high/read_only、空materials及无repository/directory/output_schema也核验。

原候选除原submit无其他执行输入；没有额外执行来源、交付或PR，HEAD、工作区状态及材料实际差异保持基线。全局服务收尾及所属资源清理在这些业务断言之后执行。执行器缺六份公开身份/唯一提交计数/非空JSON正文、两份浏览器证明或两张非空截图，以及缺场景清理/资源证据时均不通过；缺失、损坏和重复提交证据的负例由controller测试覆盖。

实际UI缺陷修复：候选页面已挂载后，从活动中心导航到有效advisory目标，原useState初始化不会再次展开面板。现在响应目标变化展开，普通重渲染保留用户后续折叠；新增导航单测及L4均通过。除此之外，本次没有生产功能扩展，没有Runner源码修改。

### 固定提交与结果

最终功能及测试源码为 `04724814a82b3a4b651b62cd2c441300b36a53ca`。以下最终四类正式运行前后均为干净工作区；本节及索引/安排收尾不冒认为新受测源码。系统临时输出根仍为 `C:/Users/lmas1/AppData/Local/Temp/`。

| 正式范围 | 本次独立目录 | 准备 / 执行 | 结果 |
| --- | --- | --- | --- |
| l1.minimum | `steerline-advisory-l4-L1-final2` | 0.4 / 21.8秒 | 通过 |
| smoke.delivery（L2） | `steerline-advisory-l4-L2-final` | 4.9 / 127.2秒 | 通过、清理完成 |
| runtime.main-agent-advisory，第1轮 | `steerline-advisory-l4-run7` | 10.9 / 210.7秒 | 通过、1项非零、零skip、清理完成 |
| runtime.main-agent-advisory，第2轮 | `steerline-advisory-l4-run8` | 9.3 / 169.9秒 | 同一提交再次通过、清理完成 |
| runtime.async-activities（L3） | `steerline-advisory-l4-activities-final` | 7.9 / 311.8秒 | 观察及显式反馈两段通过、清理完成 |

[04724814正式历史](../records/04724814a82b3a4b651b62cd2c441300b36a53ca.json)保留两轮L4与L1/L2/L3。实际证据在每轮 `scenarios/runtime.main-agent-advisory/` 的 `advisory-proof.json`、`advisory-public-requests.json`、`browser-ready/`、`browser-cancel/`、`scenario.json` 和所属Runner清理记录；两份浏览器证明分别只读/复制零命令和唯一main_agent_cancel。每轮六个作业各Submit一次，原开发Run在业务断言时未停止。

整合main后的 `3546519a` 已通过Web全集389项（执行20.6秒）和controller/fixture/testenv完整定向286项（45.1秒），目录分别为 `steerline-advisory-l4-web-final`、`steerline-advisory-l4-owner-final`；后续仅改本场景的等待/读取，不改已通过的UI或controller。较早owner1登记旧runtime列表漏新ID而失败，owner2修正后286项通过（51.9秒）；原报告均保留，不能当全集失败被隐藏。

### 保留的失败与修正

| 目录后缀（共同前缀 `steerline-advisory-l4-`） | 实际结果及后续处理 |
| --- | --- |
| run1 | 失败：测试在默认“进行中”查已完成建议，补切“需处理” |
| run2 | 失败：捕获上述同候选深链不展开的真实UI缺陷，修复并补回归 |
| run3（3546519a） | 失败：应用先保存unknown后发起Submit的合法窗口中，测试过早断言计数；改为对端接收和查询事实屏障 |
| run4（95229989） | 失败：故障脚本保持查询不可用导致既有启动恢复正确阻断；改为停止宿主后恢复查询再启动 |
| run5（fd8252a2） | 失败：Windows原子替换时外部快照读取sharing violation；改为有界等待可读，解析损坏仍失败，不重置或写夹具 |
| run6（冻结276068b5） | 首次完整通过；随后将两处浏览器前后快照读取也统一到有界读取，不作为047最终版本证明 |

这些轮次清理均完成。前三个干净失败版本分别导出正式历史：[3546519a](../records/3546519a63746249cc4a263d45f51d3c94967eeb.json)、[95229989](../records/952299893b3517f589b0e055a72f23e49fb2782f.json)、[fd8252a2](../records/fd8252a291efcff1ab723ea8dd8bfd98111d667b.json)。dirty诊断、冻结构建及最终版本分别记录，不用重试覆盖原输出。

### 本次提示词差异及独立复核

相对增补前 `4a77fcc`，实际提示词正文diff为空：模板145→145 B、完整规则1025→1025 B、golden1180→1180 B，逐字相同。本次业务原因是稳定产品接线和生命周期回归，只新增实际完整Instruction、模型及能力字段检查，不改提示词、不更新黄金样本。测试最初的三个Contains已收紧为按冻结Context完整正文相等，避免漏规则或串材料仍通过。

独立reviewer `/root/independent_review` 已复核完整测试接线、上述UI修复及等待/读取修正，并独立核对run7的原始公开请求、两份浏览器证明、截图及清理。确认六次唯一提交、unknown原Run观察、仅停止两个建议、0字节资产变化，无遗留发现；测试由当前聊天实际执行。

本项不调用真实模型、不创建外部PR、不修改安装环境；controlled的公开结算证明及实际所属进程/命名空间清理不冒称新增真实Codex资源验证。此前真实generate证据仍独立保留。使用者人工主观体验及完整真实开发任务待答闭环仍未验；不声称严格工具/隐式上下文隔离。后续main仅引起验收索引冲突，本功能索引移至独立位置解决，不为收尾引入其他功能修改。未合并、未部署。

## 2026-11-10 合并前新鲜复审

本次重新审查完整PR及当前main的组合，确实发现并修复两项P2，不沿用上一轮“未发现”结论：

| 问题 | 真实触发与后果 | 修复及新增验证 |
| --- | --- | --- |
| 普通观察使取消过期 | 后台观察每次推进记录revision，页面读取后再点取消即可能被严格相等校验拒绝；原L4取消前冻结观察，漏掉正常路径 | 作业ID/输入始终不变，取消接受历史正revision；0/未来值及成功/失败终态仍拒绝。静态与实际进度两类红测先复现，再验证修复；浏览器暂停原POST直到显示版本过期，再真实提交 |
| 未知stop无法核实 | CAS过期而拒绝的停止回执又丢失时，只查原Run不会识别未受理信封，可能持续cancelling并占槽/阻断启动 | live/startup共用公开原stop查询：先读已核实Run，再查原请求；缺失且旧CAS小于单调Run版本才永久stale，先持久拒绝再换信封；同版未知和已接受停止不重发。补错绑定、查询失败、启动恢复及fresh stopped不额外停止的测试 |

这不扩展模块能力。contract/core、提示词及Runner源码均未改；生成仍只有原提交一次，取消/停止仅面向原建议资源。实际提示词diff为空，模板145→145 B、规则1025→1025 B、golden1180→1180 B，无样本更新。

修复提交 `cdf5bd77ec640ccb65ddd11f6572d18ded6fe0f8` 已无冲突整合当时main `d1fe6f77`。以下正式运行均绑定该干净受测源码；之后只收尾本说明、Spec状态及正式历史，不冒认文档提交重新执行了这些用例。

| 正式范围 | 系统临时目录（根同上） | 执行时间 | 结果 |
| --- | --- | --- | --- |
| l1.minimum | `steerline-advisory-review-L1-20261110` | 32.1秒 | 通过 |
| smoke.delivery（L2） | `steerline-advisory-review-L2-20261110` | 90.0秒 | 通过 |
| runtime.main-agent-advisory | `steerline-advisory-review-L4a-20261110` | 160.1秒 | 通过 |
| web.unit（400项） | `steerline-advisory-review-web-20261110` | 22.5秒 | 通过 |
| go.http | `steerline-advisory-review-http-20261110` | 51.0秒 | 通过 |
| go.deployment | `steerline-advisory-review-deployment-20261110` | 500.5秒 | 通过 |
| runtime.async-activities | `steerline-advisory-review-activities2-20261110` | 288.9秒 | 通过 |

[本次正式历史](../records/cdf5bd77ec640ccb65ddd11f6572d18ded6fe0f8.json)保存七项完整运行，无skip，E2E所属资源清理均完成。HTTP压力、部署及新增L4串行；最后L2与单场景L3在隔离现场并行，总并发最多二。L3最初显式workers参数被入口拒绝、未执行测试，去掉不适用参数后使用新out运行，未把参数拒绝算场景失败或通过。

定向证据另存 `steerline-advisory-review-red-20261110`（两问题修复前确实失败）、`steerline-advisory-review-fix1-20261110`（bridge修复通过，旧启动夹具未按selector分别返回原submit/stop查询而失败）、`steerline-advisory-review-fix2-20261110`（55项通过，70.6秒）及 `steerline-advisory-review-controller-20261110`（35项通过，7.1秒）。这些是dirty诊断，不冒充正式分级全集。修正旧夹具只使用既有公开inspection映射，没有写生产状态或放宽绑定。

新版L4已去掉取消前的Unavailable冻结，公开peer在首次stop尝试之前持续产生正常进度。浏览器证明包含cancel_after_progress；公开请求证据包含两份不同的stop信封、对首次缺失请求的查询及第二份accepted，仅原建议的已受理stop允许在取消窗口新增一次。控制器还要求cancel_after_progress/unknown_stop_recovered事实及原请求/新停止身份，缺证据、不查原请求或替换已接受停止均失败。六个作业各Submit一次、业务/工作区不变、未结算占槽、source解除与清理断言保留。

独立reviewer `/root/independent_review` 新鲜审查确认上述两项，随后复核生产修复、强化L4及原始公开请求/浏览器/清理证据，结论为两项闭合、无剩余确定问题。实际测试由当前聊天执行。远端cdf5bd77的[prompt-governance](https://github.com/erranliu/SteerLine/actions/runs/42744261976/job/129592486791)也已success；旧4f96b677检查曾15分钟超时cancelled且未能取得日志，不作为当前通过依据。

人工主观体验及完整真实开发任务产Question仍未验；本次不新增真实模型调用，不改变原生工具/隐式上下文的既有边界。满足本轮合并前检查，实际合并状态以PR #478为准，未部署。
