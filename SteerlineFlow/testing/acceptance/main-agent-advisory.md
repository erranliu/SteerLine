# 主 Agent：generate 接入验收

状态：按 2026-11-09 用户确认，复用现有 Runner generate/1，本地 Codex 只读生成；来源为开发候选明确待答问题（包括普通待答）。核心独立且唯一业务产物仍为后续对话提示词，采用与执行在外部。未合并、未部署。正式目标见 [Spec](../../spec/main-agent-advisory.spec.md)、[Plan](../../plans/main-agent-advisory.plan.md)及[测试选择](../../plans/main-agent-advisory.tests.json)。

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
| A08 | 默认模型与设置冻结；最小只读上下文/正文、复制及取消；未结算继续轮询 | Web 全集 352/352 通过（本轮），人工浏览器体验未验 |
| A09 | 同包固定材料、缺失失败、实际全文 diff/字节/hash/golden/发送守卫 | 独立静态审核通过，最终证据已独立复核通过 |
| A10 | advisory-v1 固定，不接受未知协议/能力或旧动作产物 | 定向通过 |
| A11 | 原 owner 持久当前 Question/Attempt/CanReply 自动登记，不依赖诊断文案或虚构原因码 | 真实 SQLite 候选保存→后台→生成→活动→重启去重集成通过；原生问题由公开 fixture 提供，未跑完整真实开发任务 |
| A12 | 单一正式活动来源、原请求/精确导航、attention、纯读与有界正文剥离 | 集成与投影定向通过，SQLite 全集串行复核通过（本轮） |
| A13 | 不打开页面亦推进、跨候选隔离、来源解除不改历史、重启不重投 | 集成与定向通过；真实浏览器手工体验未验 |

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
