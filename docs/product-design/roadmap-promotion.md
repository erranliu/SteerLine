# Roadmap 读取、解析与入图

Web 的项目导航提供 Roadmap 页面。当前实现读取 `SteerlineFlow/Roadmap/drafts/` 下的普通 UTF-8 Markdown，保留工作树原文、摘要、HEAD 与未提交标记；不创建或改写草稿，不要求用户提供 JSON。

解析通过 Runner `generate/1`，固定完整原文、已有正式 Task/Spec/DAG 和本来源已有映射。返回内部代号、目标/范围/约束/通过条件、逐字依据、原有和建议依赖、变化理由与待澄清问题。SteerLine 校验引用、环、来源和已有关联；关键问题阻止确认。确认时重新核对原文、图及材料摘要，变化要求重新解析。新任务此时没有正式编号。

## 固定版本文档生成

2026-10-27 已接入 Runner generate/1 的 repository={local_path,commit}，根据所选构建的公开 capability/schema 开放按钮。Runner 从本地 Git 对象建立固定提交的只读快照；SteerLine 冻结原 Markdown、已确认规划、完整 commit/tree、SpecGuide 与测试/编写规范。没有能力、规范或原文/图材料变化时明确阻止。

文档生成是 purpose=documents 的独立内容记录，通过 parent_id 关联原确认，复用现有队列、原请求、查询、停止及接管。原解析和文档 Run 不混用；排队/未知/失败可观察，不自动换号重发。

逐任务 Spec/Plan/验收文件的内部代号、稳定语义路径和完整集合由输出 Schema 固定。结果还包含整批设计、调查引用和问题；调查引用按公开版本 manifest 核对路径及 SHA-256。manifest 证明固定版本提供了哪些文件，不能当作模型逐文件读取或结论正确的证明。存在关键问题时只保存问题，回到外部修改 Markdown 并重新解析确认，不通过正文暗改 DAG。

整个 Runner 最终 JSON 限 1 MiB；完整结果通过公开 content_result.json 读取。仅原生来源核实、Run completed、stop_status=stopped、Run/结果仓库 manifest 一致且业务校验通过后进入文档审阅。页面展示提交、整体设计、调查引用及 Spec/Plan/验收正文；支持受控修改，并在发布前展示所有原/新文件和目标。

## 发布与编号

文档使用稳定语义路径和 Roadmap 内部代号。发布保存原意图、审阅摘要、文档分支、固定提交、目标和合并授权，在独立 worktree 准备文档 PR；核实 provider 的原 PR、仓库、base/head、真实 merge commit 和合并后的文件。用户可授权创建并合并；不强制绕过保护规则。文档已合并但未入图时仍没有新正式编号。

正式入图经既有 Task owner 的批次事务：按仓库分配未使用的新编号，转换内部依赖，写入 Task/Contract/DAG、来源映射和原回执。一批全提交或全回滚；失去回执显示待核实，不能按页面行数猜部分创建。重新解析保留该来源内部代号已绑定的正式 Task，禁止换绑或再创建；不同草稿的同名内部代号互不冲突。

入图后沿当前派发规则运行。页面在提交前提示项目开启自动派发时可能启动任务。

## API 与原请求

查询沿已有项目 HTTP / Web 转接，不接受额外查询参数，不运行模型或发布：

| 查询 | 内容 |
| --- | --- |
| `GET /roadmaps` | 来源/历史摘要、合格内容方案和文档能力缺口 |
| `GET /roadmaps/sources/{source_id}` | 工作树 Markdown 原文 |
| `GET /roadmaps/{record_id}` | 原快照、解析/确认、文档、发布及正式映射 |

写操作均为 `POST /commands` 的原 `CommandEnvelope`。项目和实例校验沿现有协议；Roadmap 共用 HTTP 持久请求号仲裁及 SQLite 命令权威，不与派发控制/原生查看复用请求号。

| action | target / expected_version | payload |
| --- | --- | --- |
| `roadmap_parse` | source_id / 1 | `{request_id,path,source_digest,profile}`，request_id 等于信封 |
| `roadmap_confirm` | parse_id / 当前记录 revision | `{}` |
| `roadmap_generate_documents` | 已确认 parse_id / 当前记录 revision | `{}`；返回新的文档 record_id |
| `roadmap_edit_documents` | parse_id / 当前记录 revision | `{local_key,kind,body}`；kind 为 spec/plan/acceptance，未发布结果可修改 |
| `roadmap_publish` | parse_id / 当前记录 revision | `{review_digest,merge}`；固定本次发布与合并授权 |
| `roadmap_admit` | parse_id / 当前记录 revision | `{}` |
| `roadmap_observe` | parse_id / 1 | `{}`；仅查询原操作 |
| `roadmap_stop` | parse_id / 1 | `{}`；保存该记录原内容请求的停止义务 |
| `roadmap_resume` | parse_id / 当前记录 revision | `{}`；显式继续原意图中可证明尚未提交的阶段 |

回执保留原信封和 Roadmap 投影。相同请求换正文拒绝，重复请求不再次调用模型、发布或分配编号。内容请求先持久化、后提交，未知只查原请求；取消后迟到接受仍停止原 Run，原生资源未停止不能释放共享额度。启动、换构建和关闭纳入现有 Runner 原请求接管/停止核验。

发布未知只查原分支/PR。显式继续可以恢复本地准备、已核实推送后的尚未提交 PR 或已核实 PR 后尚未提交的授权合并；未知 POST/merge 不重发。Task 原回执不存在时，显式继续重检前置，仍以保存的原批次及原 Task 请求执行幂等事务。

实现与验证范围以 [专属验收](../../SteerlineFlow/testing/acceptance/roadmap-promotion.md) 为准。真实 Runner/Codex 生成到隔离 Git/SQLite 入图已通过；正式浏览器整链与真实 GitHub PR 发布合并/入图/正常停服及清理已通过；人工体验与安装部署未验。
