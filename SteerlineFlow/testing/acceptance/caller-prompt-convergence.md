# 调用方提示词收敛验收记录

关联 [Spec](../../spec/caller-prompt-convergence.spec.md) / [Plan](../../plans/caller-prompt-convergence.plan.md)。2026-10-28 用户授权实现；定向检查及 L3 已通过，最终干净提交的 L1/L2 待执行，不以本记录代替报告。

## 实现与范围

实现基于 main `ab66befe2ae4bf13de3e580d62ca2e39d51c84df`；PR399 调查版本为 `48ad01467976aff768c11f5deaaf521efbe717ee`。仅整合本次需要的完整 writer、逐 ID 接线约束、发布/报告只读边界及规范/Plan 必跑并集，不合入 PR399 其他业务改动。未修改 Runner、存储 schema、模型执行入口、预算、旧运行或安装现场。

完整安排仍原样保存，紧凑提示包括有序 ID、来源、定义摘要和原快照摘要；准备阶段从固定 Git blob 核对 Plan、规范、逐项完整定义及 note。writer 只允许 CRLF/LF 等价，缺失或不同源仍完整内联。Roadmap 规范正文保留在冻结记录用于 manifest 校验，模型通过既有固定仓库读取；原 Markdown、确认规划及其他生成的必要 inline 材料不变。环境派生事实仅省去成功项冗长 message，原报告及全部失败、未完成、PR 未知/清理事实保留；阶段从原结果初始 JSON 取关联字段。

## 自动验收

| 验收项 | 已验证范围 | 当前结果 |
| --- | --- | --- |
| CPC-A01 | 完整快照不改、有序 ID、定义/note/digest/Plan/commit 不一致及必需文件缺失拒绝、mutable checkout 不影响固定来源 | 定向及 L3 通过 |
| CPC-A02 | 无 helper 完整源码、同源/LF/CRLF 路径引用、不同源完整内联、逐入口约束；SQLite 拒绝错误 writer 分发；公开/用户正文保持、发布/报告只读 | 定向通过 |
| CPC-A03 | 测试材料和输出 schema 保持；Roadmap 非快照原文和确认输入完整，规范 manifest 缺失/错摘要拒绝，真实 Git 受控文档生成/发布/入图链保持；环境 Report 结构说明完整 | 定向通过 |
| CPC-A04 | 长成功明细投影、全部检查/L2 的 ID/状态、完整失败/incomplete 原因、PR URL/未知/清理、无报告诊断、原报告不变及确定性；prepare 保留诊断，publish 不重复前轮问题 | 定向通过 |
| CPC-A05 | Unicode、多问题/选项/自由文字原样传递；已冻结环境请求不重准备/重发；原候选 CAS/恢复、L3 重启不增加业务输入 | 定向及 L3 通过 |
| CPC-A06 | 同材料 UTF-8 组成测量、有/无 writer、多 ID 长 note、长前轮报告及固定仓库规范；必要大输入保留 | 测量通过 |
| 固定回归 | L1 `l1.minimum`、L2 `smoke.delivery` | 待执行 |

独立证据根：`C:/SteerlineDebug/cpc-9588/`。每次命令的完整参数及受测源码状态见该次 summary，原始日志不进仓库：

- `diagnostics/core-02/summary.json`：`./internal/testmanagement/... ./internal/runnerclient ./internal/storage/sqlite -count=1`，210 pass / 3 预期 opt-in skip，30.214 秒；三个 skip 为真实公开环境检查/result 往返/Runner CLI 绑定专项，本次不执行 live，不能算通过。
- `diagnostics/generation-01/summary.json`：Roadmap、app 的选定 44 项通过，包含真实 Git 受控流程及固定源反例；该次环境包编译失败，整次状态为失败，不能登记为整体通过。
- `diagnostics/environment-02/summary.json`：修正测试调用名称后，环境 owner 全部 30 项通过，15.735 秒。
- `diagnostics/answers-01/summary.json`：原生答案完整性用例通过，2.532 秒。
- `diagnostics/semantics-01/summary.json`、`diagnostics/measure-02/summary.json`：原安排、完整 writer、前轮事实、阶段身份/冻结恢复及最终安排测量通过。
- `test-results/L3/progress-03/summary.json`：`-level=L3 -suite=testing -env=controlled -scenario=testing.runtime-progress`，1 场景通过，`cleanup_complete=true`；准备 16.292 秒、执行 126.479 秒。新增紧凑输入到候选检查，保留原进度/重跑/格式错误/断连恢复/重启/原输入和清理检查。该次在提交前工作树执行。
- `python scripts/check_doc_links.py`：597 文件、3781 本地链接、0 错误；`git diff --check` 通过。

## 实际减量

以下为组件的 UTF-8 字节，不是最终模型总输入或投递稳定性结论。测试安排/选择/环境提示以前述 PR399 固定 Git blob 为比较基线；Roadmap 用 main 固定规范。未重复计算请求 JSON 中的持久化副本。

| 相同材料组成 | 整改前 | 整改后 | 保留内容 |
| --- | ---: | ---: | --- |
| 两 ID 长 note 执行上下文；同源 helper | 10893 | 2260 | 全部 ID/顺序/来源/digest，完整 5496 字节快照未改，规范/清单固定文件可读 |
| 同一安排；缺 helper | 10893 | 6004 | 仍完整提供正式 3253 字节 writer 及接线要求，不为减量省源码 |
| 测试选择指令 | 988 | 415 | 规范与 Plan 必跑并集、影响追加、空选择例外、原 schema 和全部 inline 材料 |
| 环境生成固定说明（Git blob LF） | 5896 | 5570 | 完整环境 Report 字段/语义、L2、真实 PR、用户准备及清理约束 |
| 长前轮原报告 / 派生事实 | 15675 | 740 | 全部 ID/状态、完整失败/未完成原因、来源与 PR 事实；原报告未改写 |
| Roadmap 规范相关正文 + inline | 1319 + 10344 | 1917 + 0 | 规范完整固定仓库内容/来源校验；原 Markdown、确认规划仍完整 inline |

规范 5919 的历史工作区数字与本次 Git blob LF 的 5896 区别来自行尾，未据此虚报语义减量。小安排或缺 helper 项目仍可能有较大输入，不设比例门禁。测量脚本与固定来源/组成保存在 `diagnostics/measurements/measure.py`、`components.json`，安排/报告合成夹具及测量日志见对应 Go 用例。

## 原失败与边界

- `generation-01` 的新增测试误调用不存在的 `Tick`，改为正式 `Observe` 后完整环境 owner 补测通过；保留原编译失败日志。
- L3 首次参数未经 PowerShell 字面引用，在参数校验阶段退出，未执行场景。正式调用对含点号参数加引号。
- `test-results/L3/progress-02` 的新增断言错误要求只发布 implementation 的受控 peer 也提供 publish 提示，等待超时；原产品候选实际已正常接受。改为核对已发布步骤，完整发布/报告权限隔离由真实 workflow 形状的 owner fixture 验证。该次失败且 cleanup 完整；新现场 `progress-03` 已通过。

本次没有新增人工必验项，未执行真实模型、Orca、Unity、真实业务 PR 或原 Task18 两轮。受控/静态结果不声称模型实际阅读、真实发布或输入投递稳定性通过。原现场及历史验收保持原状。
