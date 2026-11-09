# 主 Agent 给定上下文提示词建议：验收记录

状态：2026-11-09，按用户选择交付**模块重构 Draft PR，生产建议生成保持阻断**。完整目标见 [Spec](../../spec/main-agent-advisory.spec.md) 与 [Plan](../../plans/main-agent-advisory.plan.md)。这次阶段交付不等同于完整自动生成闭环；旧 [PR475 验收](main-agent-materials.md)只作历史记录。

## 范围与验收结论

| ID | 本次结果 | 未完成或证据边界 |
| --- | --- | --- |
| MAA-A01 | 受控通过：stdlib-only contract、无外部读取/执行的 core、宿主 owner、显式详情 DTO；依赖负例守卫通过 | 独立静态审核通过，不证明原生能力 |
| MAA-A02 | 受控通过：冻结正文/digest、材料 ID、UTF-8、未知字段、数量/字节限制及完整给定输入 | 准备失败未保存已取得的部分材料，界面仅显示诊断；来源适配缺有效 goal 或超限时的登记仍待完善 |
| MAA-A03 | 受控通过：唯一非空 prompt 校验、旧 JSON 动作产物拒绝、内存结果接收 | 生产没有模型产物；清晰/缺失/冲突材料的真实生成质量未验 |
| MAA-A04 | **未通过**：当前本机捕获仍包含工具定义，未获得强制零工具的公开保证 | 生产无模型/Runner 客户端，零业务提交；不以 read_only 降级 |
| MAA-A05 | **未通过**：受测配置仍附加 skills、权限、协作模式等材料 | 未穷举所有原生设置，不能声称原生永远不支持；阻断不变 |
| MAA-A06 | 受控通过：旧回答/发布/规则准备链删除；旧动作在任何业务访问前拒绝 | 原业务入口沿现有实现，HTTP/L2/部署回归通过；没有再次向真实候选发答案或发布 PR |
| MAA-A07 | 本地阶段通过：持久去重、重启不重开、准备中取消、迟到结果不覆盖取消、源解除 | 无原生资源启动；真实提交未知、配额及停止结算未验，待未来宿主接入 |
| MAA-A08 | 受控通过：只读上下文/正文、复制、取消；无生成/编辑/采用/执行入口；Sol/high 默认设置保留 | 仅保存模型偏好，不启动生成；失败材料不完整；人工体验未验 |
| MAA-A09 | 静态与受控通过：同包固定材料组装、无 inline fallback、正文 diff/字节/hash/golden及提示词守卫独立审核 | 无生产 on-wire 请求；1174 B 是代表性完整组装文本，不冒充真实模型输入 |
| MAA-A10 | 受控通过：advisory-v1 身份固定；未知协议/能力/字段拒绝；无工具或动作开关 | 扩权需新契约 |
| MAA-A11 | 受控通过：明确原因码适配、缓存原事实、稳定 occurrence 去重、后台准备/失败/阻断登记 | **真实生产方是否提供两类语义原因码未验**；普通 question/blocked 不触发，不宣称真实自动闭环完成 |
| MAA-A12 | 受控通过：单一正式活动来源、状态/attention、原请求关联、精确导航、只读/分页/预算/项目隔离 | SQLite 首次旧分页测试失败，定向与全集复跑通过，保留失败证据；真实用户体验未验 |
| MAA-A13 | 部分受控通过：后台独立于页面、持久去重、跨候选/记录隔离、不在打开面板时生成 | 真实无页面自动生成、完整崩溃窗口及浏览器手工操作未验 |

`ProductionAvailable()` 固定为 false，bridge 准备后无条件进入 blocked。不存在生产模型 sender，不能用设置或输出绕过；已结算仅表示未提交任何外部资源。核心 Accept 的内存测试以及 Web 的 ready fixture 不构成生产成功路径。

## 受测基线与自动报告

文档冻结提交 `4316418ba0ca854813816a042b99274c9a46539e`；实施在其 dirty worktree，合入 `origin/main` 的 `eed86e1355b14d07521c85483c511bd2e625f585` 后验证。各报告保存实际 source_commit 和 worktree_status；不是以最终提交 SHA 冒认先前测试。SteerRunner 未修改、未重新发布，本次没有真实模型调用。

以下报告根均为 `C:/Users/lmas1/AppData/Local/Temp/`，每次使用独立 scope，完整命令、原始日志和断言在各 `summary.json` 及 managed 报告中：

| 检查 | 报告目录 | 结果 |
| --- | --- | --- |
| owner 核心/宿主/app/SQLite/query/prompts/boundary 定向 | `steerline-advisory-owner-33b99aeab7ce4418be169fa053915489` | 42 项通过；contract 无测试文件仅编译，不冒认行为通过 |
| 准备中取消与核心定向 | `steerline-advisory-cancel-7e5a580862b546929a8d1ec4ef322a92` | 通过，覆盖取消及时返回及迟到结果 |
| 最终 owner 定向 | `steerline-advisory-final-owner-20261109` | 通过 |
| L1 固定最小集 | `steerline-advisory-final-L1-report` | 通过，18.2 秒；原 scope 为工作区临时 `$scope`，只在完成后移动报告目录，日志内部原路径保留 |
| L2 固定 controlled smoke | `steerline-advisory-L2-83fd72b37c054b618169125bf3f76586` | 通过，97.1 秒，清理完成 |
| HTTP 全集 `go.http` | `steerline-advisory-http-f159cf1e5ca34f478259ac1697aaf6c0` | 通过，63.4 秒；最终复核 `steerline-advisory-final-http-20261109` 通过，66.6 秒 |
| Web 全集 `web.unit` | `steerline-advisory-web-155011bf0a654d55aa5bc5d1b8a42606` | 350/350 通过；最终复核 `steerline-advisory-final-web-20261109` 通过，37.3 秒 |
| SQLite 全集首次 `go.storage` | `steerline-advisory-storage-8d7431e84ee74486aa8684ce39d7e8ed` | 失败：旧 `TestAsyncActivityPaginationAndRequestLookup` 报 association lookup depended on page |
| 原失败项独立复核 | `steerline-advisory-storage-link-181b090f64294413863a47ff5422a45b` | 通过，25.0 秒；未改断言或每来源预算 |
| SQLite 全集复核 `go.storage` | `steerline-advisory-storage-recheck-8d605cfdbdb04e859c59bdef9ecbf300` | 通过，85.5 秒；失败原因未被确证，不以推测掩盖首次失败 |
| 部署全集 `go.deployment` | `steerline-advisory-deployment-1f5343857a814b1baa306964e8ef1edf` | 通过，557.1 秒 |

正式命令形式：`go run ./cmd/test-feedback -level=L1/-level=L2 -out=<独立目录>`；owner 通过 `--` 传具体包及 TestAdvisory/主 Agent/活动过滤，详见报告 command；全集使用 `-level=L4 -suite=go -scenario go.http/go.storage/go.deployment` 或 `-suite=web -scenario web.unit`（每次一个场景）。PowerShell 原生参数须引用带点场景字符串。未启动成功的参数解析尝试不登记为测试通过。

`npm --prefix web run build` 与 TypeScript 编译通过。`roadmap_publication.go` 已恢复至 origin/main，Roadmap 实现/提示词/守卫相对本 PR base 无改动，故不重复跑 `go.roadmap`。本 PR 的 main-agent 固定资产与边界守卫由 L1/owner 覆盖；main 上已合并的其他提示词改动不计入本用途审核。

## 原生前置调查：未通过

公开原生接口参考 [app-server](https://learn.chatgpt.com/docs/app-server) 和[配置参考](https://learn.chatgpt.com/docs/config-file/config-reference)，并核对本机生成的普通及 experimental schema。ThreadStart/TurnStart 未发现 toolChoice/allowedTools 字段；`dynamicTools=[]` 不能证明内建工具关闭。

- npm Codex `0.166.0`，EXE SHA-256 `9d563ee4e0441ff9d59d612013e0460ddf8e5709735c2d9f6b98b7ef8ca4eb42`。
- desktop Codex `0.167.0-alpha.3.2`，EXE SHA-256 `4b3286e35d7fe866c6b0eb3f07d7a4cdb5e96489576170dfd6b3a15aa1c67382`；分别核对 schema，下面最终干净配置捕获仅针对 npm 版本。
- 用隔离本地 fake Responses provider 捕获请求，不访问真实模型。清空父级 CODEX 环境、使用新 CODEX_HOME，关闭 shell、code_mode、multi_agent、apps/plugins/hooks 等开关，`project_doc_max_bytes=0`。有效配置及原始请求保存在 `steerline-advisory-surface-9ivroga_`。
- 顶层没有 `tools`，但 `input.additional_tools` 中有两个 namespace、递归 **10 个叶子工具**，包括 functions.exec/wait/用户输入及 collaboration 操作，`tool_choice=auto`。只数顶层字段或两个 namespace 会误报零工具。
- 同时存在 skills 元信息、权限、协作与多 Agent 规则、环境上下文。该配置的 include_permissions_instructions/include_collaboration_mode_instructions 仍为 true、skills 为 null；不声称已经穷举原生配置或证明永不可能支持。

[脱敏调查摘要及原文件 hash](evidence/main-agent-native-gate.json)可供核对；完整原始捕获留在本机临时目录，包含个人环境材料，不提交仓库。该证据证明受测组合未达到保证，不能作为“已强制零工具”的正向证据。没有获得可发布的 Runner capability，也没有模拟成真实生成成功。

## 提示词实际变更与独立审核

[完整正文与接线差异](evidence/main-agent-advisory-prompts.diff)以 PR475 的 `e9ee368f` 为基准，包含模板、规则、代表性组装 golden、组装/测试接线、旧项目规则删除与治理例外。业务原因：模型不能读文件，必须由宿主在调用前完整组装；唯一输出收敛为后续对话提示词，删除有工具调查、答案和文件变更指令。没有读取失败时的 inline 兜底。

| UTF-8 资产 | PR475 之前 | 本草稿之后 | 相对 origin/main |
| --- | ---: | ---: | --- |
| `internal/prompts/main-agent.md` | 227 B | 145 B | 新增，0 → 145 B |
| `internal/prompts/materials/main-agent.md` | 2930 B | 1019 B | 新增，0 → 1019 B |
| `internal/prompts/testdata/main-agent.golden.md` | 226 B | 1174 B | 新增，0 → 1174 B |
| `SteerlineFlow/main-agent-rules.md` | 2930 B | 删除 | main 原无此资产，净无新增 |

完整规则 SHA-256：`79e76b91c5ec53c9d7d6cb5df33483da62822a6667b2286944476e51cb10ecdc`。1174 B golden 为 goal=clarify、空材料的固定完整组装正文；动态请求实际长度随上下文变化，本草稿没有生产模型输入。黄金样本通过不能替代批准。

独立 reviewer `/root/independent_review` 已审核实际正文 diff、字节、hash、同包组装/缺失失败、依赖及旧出口删除，结论为“可按生产生成保持阻断的 Draft 范围通过独立静态审核”。审核发现准备中取消不能及时生效，修复为 context-aware gate + 每记录取消，测试验证迟到结果不覆盖取消，复审通过。审核者未运行测试，自动结果由当前聊天执行，职责不混淆。

## 保留的完成条件

完整 Spec 仍需：可验证的无工具/无隐式上下文 Runner 公开能力、真实来源生产方、真实模型及停止/未知资源验证、准备失败时部分材料可见，以及人工浏览器可读性/导航/复制检查。没有人工验收通过记录，没有部署或合并。开放能力必须经后续实现与真实验证，不是解除当前硬编码阻断即可完成。

最终文档独立复核亦通过：证据 diff 与实际差异一致，字节/hash及脱敏摘要的四份原文件 hash 均已独立核对；未批准开放生产推理。最终 Web 生产构建通过（2195 modules）。本阶段文档导航检查：722 个 Markdown、4461 条本地链接、0 错误；`git diff --check` 通过。
