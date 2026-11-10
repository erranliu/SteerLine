# Roadmap 读取、解析与入图

当前 Web 读取 SteerlineFlow/Roadmap/drafts/ 下普通 UTF-8 Markdown，提供解析、确认、文档生成/审阅、发布与正式入图。草稿由外部维护，产品不改写原文或要求手写 JSON；完整项目添加仍是[后续方案](project-onboarding.md)。

## 材料与解析

解析通过 Runner generate/1，固定工作树原文/摘要、HEAD/未提交标记、已有正式 Task/Spec/DAG 与本来源映射。返回内部代号、目标/范围/约束/通过条件、逐字依据、原有/建议依赖、理由与待澄清问题；SteerLine 核对来源、引用和环。关键问题阻止确认，确认前重核原文、图与材料，变化要求重解析。此时不分配正式 Task 编号。

草稿前置可混合写本草稿代号和当前项目正式任务编号，例如 `T2 前置：T1、#8`；也接受明确标注的“正式任务 8”，含义不明的裸数字须澄清。解析分别保存 `local=["T1"]`、`formal=["8"]`，正式前置无需在草稿重列任务，“依赖 #8”也不会将当前条目关联成 #8。原依赖保留供审阅，建议关系将已绑定正式任务的草稿代号归一化并去重；语义变化须说明理由。原关系与建议关系都校验引用，未知/其他项目/已丢弃任务、自依赖和合并图中的环阻止确认，已有正式任务的前置保持原状。页面显示正式编号、固定快照中的名称和 DAG 链接；未完成的正式任务允许引用，运行状态进展不使规划失效，实际启动资格仍由 Task 调度判断。入图事务再次按当前正式图校验。

静态规范通过固定 Git repository 提供；原 Markdown、任务图、来源映射与确认规划由原请求自动冻结，通过 Runner 正式 kind=file 私有只读渠道交付，不提交动态 JSON，也不将正文内联进提示词。新请求须有 frozen_private_files/v1；缺能力或材料阻断。恢复只使用原请求已保存文件，不读取当前值重建。

原始字节与摘要保留；草稿修改标记、跨阶段内容比较允许 LF/CRLF 差异。Run/结果 manifest 须匹配原仓库与上传字节，证明材料来源，不证明模型逐文件已读或结论正确。具体供给边界归[提示词治理](../prompt-governance.md)。

## 文档生成与审阅

确认后的解析记录派生 purpose=documents 的**独立文档记录**，parent_id 指向原解析；两者分别持有 Run、revision、原请求与停止义务，不混用目标。文档生成固定完整 commit/tree、原规划、SpecGuide 与测试/编写规范，能力按所选 Runner 公开 schema 校验。

输出 Schema 固定内部代号、稳定语义路径及完整 Spec/Plan/验收文件集合，同时返回整体设计、调查引用和问题。引用核对公开 manifest 路径/SHA；关键问题回到外部草稿修订并重新解析确认，不靠编辑正文暗改 DAG。

每项文档的 tests 保留有序唯一 ID；provides_tests 声明本任务将新增并登记的 ID，无新增时为空数组。同一 ID 只能有一个批内提供者。当前清单没有的 ID 只允许被该提供者的严格依赖后继（含传递后继）选择；提供者不能用自己尚未交付的 ID 作为自身运行安排。Plan 写清提供者与交接，发布的 generated 索引和审阅摘要保存声明，正式 .tests.json 仍只保存 tests。发布预览区分已登记定义与 pending_tests，不提前伪造定义。生成遇到冲突返回最终 JSON questions 和空 documents，不调用交互提问。

完整最终 JSON 上限 1 MiB，从公开 content_result.json 读取。原生来源已核实、Run completed、stop_status=stopped、仓库 manifest 一致且业务校验通过后进入审阅。页面展示来源提交、整体设计、引用及完整正文，允许修改未发布的文档记录；发布前展示所有原/新文件及目标。

失败保留 ContentResult、Run 和 Final 的具体诊断；human_input_required 另从公开 history 核对原 Run/input/revision、Command/session/turn 和分页，显示原问题正文。详情无法核实时保留失败码及读取诊断，不将其它执行的问题挂到当前记录，也不据历史问题产生回复或恢复资格。

## 发布与编号

发布固定原请求、审阅摘要、文档分支/提交、目标及合并授权，在独立 worktree 准备文档 PR。核实 provider 的原 PR、仓库、base/head、真实 merge commit 和合并后文件；可按用户授权创建及合并，不绕过保护。文档合并尚未入图时没有新正式编号。

正式入图由 Task owner 在批次事务分配新编号、转换内部依赖，写 Task/Contract/DAG、来源映射与原回执；整批提交或回滚。未知只核实原批次，不按页面行数猜部分成功。重新解析保留本来源内部代号到正式 Task 的绑定，不换绑/再创建；不同草稿同名代号互不冲突。

入图不绕过[项目统一就绪与派发资格](project-maintenance.md#项目统一就绪与候选冻结)。页面提示自动派发可能启动合格任务，解析/生成/发布本身不等于开发任务已经启动。

## API 与原请求

路由相对应用 v2 项目根，通用信封及浏览器恢复归[本机协议](local-client-protocol.md)。

| GET | 内容 |
| --- | --- |
| /roadmaps | 来源/历史、合格方案、文档能力及静态准备状态 |
| /roadmaps/sources/{source_id} | 工作树 Markdown 原文 |
| /roadmaps/{record_id} | 原快照、解析/文档、发布及正式映射 |

GET 不接受额外查询参数，不启动模型或执行发布。写操作统一 POST /commands，核对项目/实例并保留完整原信封；HTTP 请求号与 SQLite 权威共同仲裁，不与派发控制或原生查看复用 ID。

| action | target / expected_version | payload |
| --- | --- | --- |
| roadmap_prepare_static | project_id / 1 | {commit}，页面观察的本地完整 HEAD |
| roadmap_parse | source_id / 1 | {request_id,path,source_digest,profile}，request_id 等于信封 |
| roadmap_confirm | 解析 record_id / 当前 revision | {} |
| roadmap_generate_documents | 已确认解析 record_id / 当前 revision | {}；返回新的文档 record_id |
| roadmap_edit_documents | 文档 record_id / 当前 revision | {local_key,kind,body}；kind 为 spec/plan/acceptance，仅未发布结果 |
| roadmap_publish | 文档 record_id / 当前 revision | {review_digest,merge} |
| roadmap_admit | 文档 record_id / 当前 revision | {} |
| roadmap_observe、roadmap_stop | 原解析或文档 record_id / 1 | {}；查询原操作或保存原内容 Run 停止义务 |
| roadmap_resume | 原文档 record_id / 当前 revision | {}；显式继续可证明未提交的发布/入图阶段 |

同 ID 改正文拒绝，重复请求不再调用模型、发布或编号。内容请求先保存再提交，未知只查原号；取消后迟到受理仍须停止原 Run，资源未停止不释放共享额度。启动/关闭核对原请求与停止义务，换构建遵守[当前排空规则](../integration/steerrunner.md#停止重启与升级)。

发布未知只查原分支/PR。显式继续可恢复本地准备、核实推送后尚未提交的 PR 或核实 PR 后尚未提交的授权合并；未知 POST/merge 不重发。入图回执缺失时，显式继续重检前置，以保存的原批次与原 Task 请求执行幂等事务。领域动作与目标以[路由实现](../../internal/app/roadmaps.go)为准。

## 静态材料准备

| 入口 | 行为 |
| --- | --- |
| Web“初始化静态材料” | 显式提交 roadmap_prepare_static，绑定当前项目与观察到的 HEAD |
| CLI | steerline project prepare-roadmap -repository <绝对目录>；-check 只检查 |
| 独立调试包 | 新项目首次提交前调用同一准备 owner |

三者共用[准备实现](../../internal/roadmap/prepare.go)：补缺少的 parse-rules.md、document-rules.md 与 SteerlineFlow/spec/SpecGuide.md；保留已有自定义 SpecGuide，平台规则不同源需审阅更新，不静默覆盖。只写静态材料，不调用模型、提交 Git、纳管、建库或建任务。

页面分别展示 HEAD 与已观察目标分支的三份材料状态。生成到工作树不等于就绪，须按项目正常流程提交/合入并同步；未发布、版本不符或目标未核实时禁止解析。GET 只观察，不 fetch 或自动初始化。

未决初始化按原号只读核实；中断后不重复文件写入，完整属性可核实则补收结果，部分结果保留并明确终止，需用户刷新后显式重新初始化。身份、原 HEAD 和写入前后仓库状态须匹配，见[应用核验](../../internal/app/roadmap_preparation.go)。

动态文件逻辑名为 roadmap-inputs/source.md、existing-task-graph.json、existing-roadmap-source-map.json、confirmed-planning.json；实际读取位置由 Runner 返回，与仓库同名文件无关，不加入项目初始化清单。

## 验证与历史

当前实际范围见[转正验收](../../SteerlineFlow/testing/acceptance/roadmap-promotion.md)；静态准备见[初始化 Spec](../../SteerlineFlow/spec/roadmap-static-initialization.spec.md)及[验收](../../SteerlineFlow/testing/acceptance/roadmap-static-initialization.md)。原受测构建的真实 Runner/Codex、浏览器、GitHub PR 到入图链路不等于当前安装已部署；人工体验和安装部署未验范围仍按原记录保留。精简前日期增量及原文从[第六批历史入口](../../SteerlineFlow/archive/design/docs-sixth-batch-review.md)追溯。
