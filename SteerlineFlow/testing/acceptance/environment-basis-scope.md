# 环境资格依据收窄与具体失效原因

2026-11-09。用户授权直接实现：只让有效要求参与环境资格摘要，收窄本项目的依赖依据，显示具体失效原因。本次在 worktree `C:\Users\lmas1\.codex\worktrees\0e78\SteerLine` 修改，基线 `eed86e1355b14d07521c85483c511bd2e625f585`，验证针对未提交的工作区；未部署或修改安装现场。

## 实现与边界

- manifest6 增加必填、可为空的非秘密 `requirements` 字符串映射，严格拒绝旧版本及非法字段。项目脚本负责解释要求，平台只固定、比较和展示。
- 摘要使用规范化机器段、脚本和显式依赖的完整字节摘要及原稳定调用上下文。说明正文、JSON 格式/键序、依赖列表顺序不使资格失效；材料缺失、不可读、超限仍拒绝复用。
- 本项目脚本直接读取同一机器段的 Go/Node/Python 最低版本，移除 `go.mod` 整文件依赖。开发回归核对 Go 要求与 `go.mod` 的 `go` 字段一致；普通 module/require 变化不失效。Git/npm 仍查询可用版本，不安装或执行测试/构建。
- 原活动冻结文件摘要；只读查询、准入预检及原观察 owner 显示一致的具体变化，包含要求新增/移除/旧值→新值、入口/解释器和具体脚本/依赖路径。最多列八项及剩余数量，缺失/非法材料保留具体读取原因及路径。
- 资格和执行结算仍按原目标独立；Current/CAS、人工撤销、新负结果优先、回退后不自动恢复、停止/封存及迟到结果规则保持。没有 TTL、自动重测、旧数据迁移或第二套资格来源。

对应规格：[环境调用](../../spec/project-environment-workflow.spec.md)、[共用材料](../../spec/shared-environment-materials.spec.md)。

## 实际正文变化

| 文件 | UTF-8 字节（前→后） | 业务变化 |
| --- | --- | --- |
| `SteerlineFlow/environment.md` | 2042 → 2368 | manifest5→6，dependencies 从 go.mod 改为空，声明三个最低版本；说明改为有效要求/具体原因及无需重测范围 |
| `SteerlineFlow/scripts/verify-environment.ps1` | 5563 → 6230 | 从同一机器段读取并校验全部最低版本，替换整份 go.mod 的读取及硬编码 Node/Python 门槛；与平台校验相同的独立围栏边界 |
| `internal/prompts/environment-invoke.md` | 412 → 412，逐字节相同 | 平台固定短提示与组装/材料交付逻辑无变化，继续只调用原项目入口 |

完整正文差异见上述文件的 Git diff。没有修改 `internal/prompts`、黄金样本、Runner 契约或 CI。项目环境说明/脚本是原业务材料，不新增模型行为规则。

## 验证

证据根为 `C:\SteerlineDebug\wt-0e78-env-basis`，每次调用使用独立叶目录。以下路径均相对该根；完整命令、实际测试选择、参数与终态见各 summary。

| 检查 | 结果 | 原始证据 |
| --- | --- | --- |
| 四根定向 Git/SQLite/PS5.1 | 通过；含正文/JSON/普通依赖复用、Go 要求具体原因、只读无撤销副作用、原来源保持及跨目标撤销/回退拒绝 | `diagnostics/44353cb0e646400b9fcc634cc84392e1/summary.json` |
| L1 固定集 | 通过 | `test-results/L1/357feb9b12824cefa0835eaa9ea3d03f/summary.json` |
| L2 受控冒烟 | 通过 | `test-results/L2/6c3ddcff0f754967b064661e08b0fabb/summary.json` |
| go.environment 完整专项 | 通过；核心包46根、App31根及全部子项，零跳过；含新增要求门槛、依赖排序和各类具体原因子项 | `test-results/L4/8dd58ee22c1a4ce09f8c0d5976be7e6f/summary.json` |
| web.unit 全集补验 | 通过，344 项 | `test-results/L4/1a091c7c41c64b02bf0093d6ab1f4590/summary.json` |
| 页面定向诊断 | 通过；环境原因展示及项目切换 | `diagnostics/b18639a1613047f1ab1c94e22ee5e20f/vitest.json` |
| Markdown 链接 / diff 空白 | 通过，718 个 Markdown、4434 条本地导航链接、零错误 | `python scripts/check_doc_links.py`、`git diff --check` |

初次沙箱内测试因 Git 无权创建夹具 `.git/config`、脚本执行受限而失败，未算通过；随后使用仓库外独立目录和正常进程权限验证。原沙箱定向报告为系统 TEMP 下 `SteerlineDebug/wt-0e78-env-basis/diagnostics/fbccdf042a3940938d494124dceb7fd3/summary.json`。最初快速模型测试还发现两个遗漏的 manifest5 解释器夹具，已同步为 manifest6，并由完整 owner 集复核。

前端全集首次 343/344 通过，原项目切换测试等待活动按钮超时；本次环境测试已通过。单独运行项目切换与环境页面均通过，随后全集 344/344 通过，未修改项目切换代码或放宽断言。原失败报告保留于 `test-results/L4/91ea6c5692e34ce7847f45263bdcdb7f/summary.json`。

本次没有运行真实 Codex/Orca Runner 或人工页面体验；隔离脚本/受控报告不代表真实链成功。其他受管项目须正常采用 manifest6 并重新检测；本次不自动改写它们。

## 实现审计与修复

2026-11-09 用户要求审计实现、关注结构债和过度设计、修复后提 PR。当前聊天完成代码/调用/存储/脚本/测试审计，不将作者自查标作独立审核。

| 发现 | 修复与验证依据 |
| --- | --- |
| 候选固定来源与当前依据不同时，一律提示“须重新检测”，且差异方向按当前→候选展示；重新检测相同当前材料无法修复来源冲突 | 先复核当前资格；有效后再比较候选，按候选→当前展示差异并提示更新候选来源。补充当前已通过新要求、旧候选仍不准入、当前资格仍有效的反例 |
| Prepared 同时保存 ScriptDigest 和 Files 中的脚本摘要，存在双重事实来源 | 只保存 Files 映射，摘要计算、差异判断和公开 script_digest 字段都使用同一冻结数据，无兼容读取或迁移路径 |
| 平台允许行内起始围栏和带后缀的结束围栏，新增的 PowerShell 读取规则与之不一致 | 两侧统一独立行上的唯一机器段，保留 LF/CRLF 和文件尾结束围栏；模型与真实 PS5.1 脚本均增加正反例 |
| 依赖差异按每个路径线性搜索已排序列表；测试固定把 Go 要求改为 1.27.0，未来正常升级会使测试失去变化 | 复用 ParseManifest 的排序，用二分查找；测试改用独立的高版本负例 |

结构取舍：保留 manifest6，因有效机器要求取代正文作为资格权威，属于明确接口语义变化；不为避免版本更新而增加旧版分支。requirements 仅为有界字符串映射，平台不引入工具类型、版本比较、投影选择器或依赖发现；项目脚本解释要求。现有 Current/CAS/Qualify 继续负责资格，变化描述只负责原因，没有新增资格状态、定时系统、自动恢复、配置注册表或框架。

审计后报告根为 `C:\SteerlineDebug\wt-0e78-env-basis-audit`，验证针对相同生产代码的工作区：

| 检查 | 结果 | 原始证据（相对审计报告根） |
| --- | --- | --- |
| L1 | 通过 | `test-results/L1/29c3b381f29f412f818908fd290656e5/summary.json` |
| L2 | 通过 | `test-results/L2/9835d2183f5d40b796fad89d3bed8df6/summary.json` |
| go.environment | 通过；核心包46根、App31根及全部子项，零失败、零跳过；App组471.1秒 | `test-results/L4/e33eeef0fd614c6ea0767f5ed90fd9de/summary.json` |
| go.http | 通过；覆盖公开报告投影、查询及 HTTP owner 全集 | `test-results/L4/22894b97dacb4259863ff3502623445f/summary.json` |

前端生产实现与页面断言语义未再修改，沿用上一节已通过的344项全集；本轮 L1 亦包含固定前端集合。提交前再次检查 Markdown 链接、正文实际字节和 diff 空白。
